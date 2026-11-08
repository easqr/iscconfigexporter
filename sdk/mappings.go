package sdk

import (
	"context"
	"errors"
	"io"

	exporter "gitlab.com/easqrtech/iam/iscconfigexporter"
	"gitlab.com/easqrtech/iam/iscconfigexporter/snapshot"
)

// GetMappings uses the existing root SDK client, gate and GET retry policy.
// Token caches remain managed by the SDK per API version.
func (a *Adapter) GetMappings(ctx context.Context, sourceOrg string) (exporter.MappingDownload, error) {
	var out exporter.MappingDownload
	if e := snapshot.ValidateTypes([]string{sourceOrg}); e != nil {
		return out, errors.New("valid mapping source organization is required")
	}
	if e := a.acquire(ctx); e != nil {
		return out, e
	}
	defer a.release()
	_, r, e := a.client.V2026.ConfigurationHubAPI.GetObjectMappings(ctx, sourceOrg).Execute()
	defer closeResponse(r)
	if e = safe(ctx, r, e); e != nil {
		return out, e
	}
	data, e := io.ReadAll(r.Body)
	if e != nil {
		return out, errors.New("cannot read mapping response")
	}
	if _, e = exporter.ValidateMappings(data); e != nil {
		return out, e
	}
	if r.Request == nil || r.Request.URL == nil {
		return out, errors.New("mapping request provenance unavailable")
	}
	out.Data = data
	out.APIOrigin = r.Request.URL.Scheme + "://" + r.Request.URL.Host
	return out, nil
}

var _ exporter.MappingAPI = (*Adapter)(nil)
