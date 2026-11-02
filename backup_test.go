package exporter

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

type backupFake struct {
	items   []Backup
	deleted []string
	fail    bool
	missing bool
}

func (f *backupFake) List(context.Context, string) ([]Backup, error) { return f.items, nil }
func (f *backupFake) Get(_ context.Context, _ string, id string) (Backup, error) {
	for _, b := range f.items {
		if b.ID == id && !f.missing {
			return b, nil
		}
	}
	return Backup{}, errors.New("gone")
}
func (f *backupFake) Delete(_ context.Context, _ string, id string) error {
	f.deleted = append(f.deleted, id)
	if f.fail {
		return errors.New("failure")
	}
	return nil
}

type registryFake []Ownership

func (r registryFake) Records(context.Context, string) ([]Ownership, error) { return r, nil }
func (registryFake) Register(context.Context, Ownership) error              { return nil }

type verified bool

func (v verified) Verify(context.Context, Backup, Ownership) (bool, error) { return bool(v), nil }
func TestRetention(t *testing.T) {
	now := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, policy, age              string
		dry, fail, unverified, missing bool
		want                           int
	}{{name: "keep latest", policy: "keep_latest", want: 1}, {name: "age boundary", policy: "older_than", age: "2d", want: 0}, {name: "older", policy: "older_than", age: "47h", want: 1}, {name: "preview", policy: "keep_latest", dry: true}, {name: "unknown", policy: "keep_latest", unverified: true}, {name: "disappeared", policy: "keep_latest", missing: true}, {name: "partial error", policy: "keep_latest", fail: true, want: 1}} {
		t.Run(tc.name, func(t *testing.T) {
			f := &backupFake{items: []Backup{{ID: "old", Tenant: "example", Status: "COMPLETE", CanDelete: true, Created: now.Add(-48 * time.Hour)}, {ID: "latest", Tenant: "example", Status: "COMPLETE", CanDelete: true, Created: now.Add(-time.Hour)}}, fail: tc.fail, missing: tc.missing}
			r := registryFake{}
			for _, b := range f.items {
				r = append(r, Ownership{Tenant: b.Tenant, OperationID: b.ID, BackupID: b.ID, Created: b.Created, Provenance: "test-only-creation-proof"})
			}
			m := BackupManager{API: f, Registry: r, Verifier: verified(!tc.unverified), Now: func() time.Time { return now }}
			report, e := m.Cleanup(context.Background(), "example", "current", BackupConfig{Enabled: true, Policy: tc.policy, Age: tc.age, DryRun: tc.dry})
			if len(f.deleted) != tc.want || (e != nil) != tc.fail {
				t.Fatal(f.deleted, report, e)
			}
		})
	}
}
func TestBackupSafeguards(t *testing.T) {
	now := time.Now().UTC()
	for _, kind := range []string{"empty", "singleton", "running", "failed", "partial", "duplicate-backup", "duplicate-registry", "no-registry", "no-verifier"} {
		t.Run(kind, func(t *testing.T) {
			items := []Backup{{ID: "old", Tenant: "example", Status: "COMPLETE", CanDelete: true, Created: now.Add(-time.Hour)}, {ID: "new", Tenant: "example", Status: "COMPLETE", CanDelete: true, Created: now}}
			r := registryFake{}
			for _, b := range items {
				r = append(r, Ownership{b.Tenant, b.ID, b.ID, "proof", b.Created})
			}
			switch kind {
			case "empty":
				items = nil
			case "singleton":
				items = items[:1]
			case "running":
				items[0].Status = "RUNNING"
			case "failed":
				items[0].Status = "FAILED"
			case "partial":
				items[0].Partial = true
			case "duplicate-backup":
				items = append(items, items[0])
			case "duplicate-registry":
				r = append(r, r[0])
			case "no-registry":
				r = nil
			}
			f := &backupFake{items: items}
			m := BackupManager{API: f, Registry: r, Verifier: verified(true)}
			if kind == "no-verifier" {
				m.Verifier = nil
			}
			_, e := m.Cleanup(context.Background(), "example", "current", BackupConfig{Enabled: true, Policy: "keep_latest"})
			if e != nil || len(f.deleted) != 0 {
				t.Fatal(f.deleted, e)
			}
		})
	}
	_, e := (BackupManager{}).Cleanup(context.Background(), "example", "", BackupConfig{})
	if e != nil {
		t.Fatal(e)
	}
}
func TestRegistry(t *testing.T) {
	r := FileRegistry{filepath.Join(t.TempDir(), "registry.json")}
	ctx := context.Background()
	v := Ownership{"example", "operation", "backup", "test provenance", time.Now().UTC()}
	if e := r.Register(ctx, v); e != nil {
		t.Fatal(e)
	}
	if e := r.Register(ctx, v); e == nil {
		t.Fatal("duplicate accepted")
	}
	records, e := r.Records(ctx, "example")
	if e != nil || len(records) != 1 {
		t.Fatal(records, e)
	}
	records, e = r.Records(ctx, "other")
	if e != nil || len(records) != 0 {
		t.Fatal(records, e)
	}
}
