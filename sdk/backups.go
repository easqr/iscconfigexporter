package sdk

import (
	"context"
	"errors"
	exporter "github.com/elianderson/iscutilities/configexporter"
	v2026 "github.com/sailpoint-oss/golang-sdk/v2/api_v2026"
)

// Backups provides read-only inventory. No Configuration Hub DELETE is issued.
type Backups struct {
	adapter *Adapter
	client  *v2026.APIClient
	tenant  string
}

func (a *Adapter) Backups(c exporter.Config) *Backups {
	return &Backups{a, a.client.V2026, c.Tenant}
}
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
func (*Backups) Delete(context.Context, string, string) error {
	return exporter.ErrOwnershipUnavailable
}
