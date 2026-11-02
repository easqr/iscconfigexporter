package exporter

import (
	"context"
	"errors"
	"sort"
	"time"
)

var ErrOwnershipUnavailable = errors.New("SP-Config to Configuration Hub ownership cannot be verified; deletion disabled")

// BackupManager is usable with platform-provided verified adapters. The shipped
// SDK inventory has no destructive implementation or ownership verifier.
type BackupManager struct {
	API      BackupAPI
	Registry Registry
	Verifier OwnershipVerifier
	Now      func() time.Time
}

func (m BackupManager) Cleanup(ctx context.Context, tenant, operation string, c BackupConfig) (CleanupReport, error) {
	report := CleanupReport{}
	if e := ctx.Err(); e != nil {
		return report, e
	}
	if !c.Enabled || c.Policy == "disabled" {
		return report, nil
	}
	if !portable(tenant) {
		return report, errors.New("invalid cleanup tenant")
	}
	if c.Policy != "older_than" && c.Policy != "keep_latest" {
		return report, errors.New("invalid cleanup policy")
	}
	age := time.Duration(0)
	var e error
	if c.Policy == "older_than" {
		age, e = Duration(c.Age)
		if e != nil || age <= 0 {
			return report, errors.New("invalid cleanup age")
		}
	} else if c.Age != "" {
		return report, errors.New("contradictory cleanup age")
	}
	if m.API == nil {
		return report, errors.New("backup API unavailable")
	}
	backups, e := m.API.List(ctx, tenant)
	if e != nil {
		return report, errors.New("backup enumeration failed")
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].ID < backups[j].ID })
	if m.Registry == nil || m.Verifier == nil {
		report.Limitation = ErrOwnershipUnavailable.Error()
		for _, b := range backups {
			report.Decisions = append(report.Decisions, Decision{b.ID, "skip", "ownership unverified"})
		}
		return report, nil
	}
	records, e := m.Registry.Records(ctx, tenant)
	if e != nil {
		return report, errors.New("ownership registry unavailable")
	}
	byID := map[string]Ownership{}
	ambiguous := map[string]bool{}
	for _, r := range records {
		if _, ok := byID[r.BackupID]; ok {
			ambiguous[r.BackupID] = true
		}
		byID[r.BackupID] = r
	}
	counts := map[string]int{}
	for _, b := range backups {
		counts[b.ID]++
	}
	for _, r := range records {
		if r.BackupID != "" && counts[r.BackupID] == 0 {
			report.Decisions = append(report.Decisions, Decision{r.BackupID, "skip", "registered backup missing from inventory"})
		}
	}
	now := time.Now().UTC()
	if m.Now != nil {
		now = m.Now().UTC()
	}
	eligible := map[string]Backup{}
	latest := time.Time{}
	protected := map[string]bool{}
	var errs []error
	// Evaluate all candidates before making any deletion request.
	for _, b := range backups {
		r, ok := byID[b.ID]
		reason := ""
		switch {
		case counts[b.ID] != 1 || ambiguous[b.ID]:
			reason = "ambiguous duplicate identity"
		case !ok || r.Tenant != tenant || r.OperationID == "" || r.Provenance == "" || r.BackupID == "" || r.Created.IsZero():
			reason = "ownership record missing or invalid"
		case b.Tenant != tenant || b.ID == "" || b.Created.IsZero() || b.Created.After(now) || !b.Created.Equal(r.Created):
			reason = "backup identity or creation metadata mismatch"
		case b.Status != "COMPLETE" || b.Partial:
			reason = "backup not fully successful"
		default:
			verified, err := m.Verifier.Verify(ctx, b, r)
			if err != nil || !verified {
				reason = "ownership not independently verified"
			}
		}
		if reason != "" {
			report.Decisions = append(report.Decisions, Decision{b.ID, "skip", reason})
			continue
		}
		eligible[b.ID] = b
		if b.Created.After(latest) {
			latest = b.Created
		}
	}
	for id, b := range eligible {
		if b.Created.Equal(latest) {
			protected[id] = true
		}
	}
	ids := []string{}
	for id := range eligible {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		b := eligible[id]
		reason := ""
		switch {
		case protected[id]:
			reason = "latest successful exporter-owned backup"
		case id == operation:
			reason = "current export operation"
		case !b.CanDelete:
			reason = "API denies deletion"
		case c.Policy == "older_than" && !b.Created.Before(now.Add(-age)):
			reason = "within retention age"
		}
		if reason != "" {
			report.Decisions = append(report.Decisions, Decision{id, "retain", reason})
			continue
		}
		if c.DryRun {
			report.Decisions = append(report.Decisions, Decision{id, "would_delete", "eligible; preview only"})
			continue
		}
		if e = ctx.Err(); e != nil {
			return report, e
		}
		fresh, e := m.API.Get(ctx, tenant, id)
		if e != nil || fresh != b {
			report.Decisions = append(report.Decisions, Decision{id, "skip", "backup disappeared or changed"})
			continue
		}
		ok, e := m.Verifier.Verify(ctx, fresh, byID[id])
		if e != nil || !ok {
			report.Decisions = append(report.Decisions, Decision{id, "skip", "ownership recheck failed"})
			continue
		}
		// Ensure a protected successful successor still exists before every deletion.
		survivor := false
		for pid := range protected {
			p, e := m.API.Get(ctx, tenant, pid)
			if e == nil && p == eligible[pid] {
				v, e := m.Verifier.Verify(ctx, p, byID[pid])
				if e == nil && v {
					survivor = true
				}
			}
		}
		if !survivor {
			report.Decisions = append(report.Decisions, Decision{id, "skip", "protected successor unavailable"})
			continue
		}
		if e = m.API.Delete(ctx, tenant, id); e != nil {
			report.Decisions = append(report.Decisions, Decision{id, "delete_failed", "deletion attempted; API failed"})
			errs = append(errs, errors.New("backup deletion failed"))
		} else {
			report.Decisions = append(report.Decisions, Decision{id, "deleted", "deletion attempted; API confirmed success"})
		}
	}
	return report, errors.Join(errs...)
}
