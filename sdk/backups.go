package sdk

import (
	"context"
	"errors"
	exporter "github.com/easqr/iscconfigexporter"
	v2026 "github.com/sailpoint-oss/golang-sdk/v2/api_v2026"
)

// Backups provides read-only Configuration Hub inventory for an Adapter.
// Obtain it with Adapter.Backups; the zero value is not usable. It implements
// exporter.BackupAPI, but Delete always returns exporter.ErrOwnershipUnavailable.
// Calls share the adapter's serialization and SDK authentication limits.
type Backups struct {
	adapter *Adapter
	client  *v2026.APIClient
	tenant  string
}

// Backups returns inventory bound to c.Tenant for local request checks. Other
// policy fields are unused. It does not contact ISC or change the adapter's SDK
// connection, and c.Tenant is not independently checked against the remote origin.
func (a *Adapter) Backups(c exporter.Config) *Backups {
	return &Backups{a, a.client.V2026, c.Tenant}
}

// List reads backup inventory and requires tenant to match the configured local
// tenant. It does not infer exporter ownership. Missing completeness metadata is
// treated as partial; missing permission is false and missing creation time is zero.
func (b *Backups) List(ctx context.Context, tenant string) ([]exporter.Backup, error) {
	if tenant != b.tenant {
		return nil, errors.New("backup tenant mismatch")
	}
	if err := b.adapter.acquire(ctx); err != nil {
		return nil, err
	}
	defer b.adapter.release()
	v, r, e := b.client.ConfigurationHubAPI.ListBackups(ctx).Execute()
	defer closeResponse(r)
	if e = safe(ctx, r, e); e != nil {
		return nil, e
	}
	out := []exporter.Backup{}
	for _, v := range v {
		record := exporter.Backup{ID: v.GetJobId(), Tenant: v.GetTenant(), Status: v.GetStatus(), CanDelete: v.GetUserCanDelete(), Partial: true}
		if v.IsPartial != nil {
			record.Partial = *v.IsPartial
		}
		if v.Created != nil {
			record.Created = v.Created.Time
		}
		out = append(out, record)
	}
	return out, nil
}

// Get lists inventory and returns the unique matching backup ID. Missing or
// duplicate IDs are errors. The lookup makes a remote inventory request and can
// trigger SDK token acquisition.
func (b *Backups) Get(ctx context.Context, tenant, id string) (exporter.Backup, error) {
	v, e := b.List(ctx, tenant)
	if e != nil {
		return exporter.Backup{}, e
	}
	var found *exporter.Backup
	for _, v := range v {
		if v.ID == id {
			if found != nil {
				return exporter.Backup{}, errors.New("duplicate backup ID")
			}
			copy := v
			found = &copy
		}
	}
	if found == nil {
		return exporter.Backup{}, errors.New("backup missing")
	}
	return *found, nil
}

// Delete always returns exporter.ErrOwnershipUnavailable without authentication
// or an HTTP request, including when the context is canceled. A local registry
// cannot enable it. Check the error with errors.Is(err, exporter.ErrOwnershipUnavailable).
func (*Backups) Delete(context.Context, string, string) error {
	return exporter.ErrOwnershipUnavailable
}
