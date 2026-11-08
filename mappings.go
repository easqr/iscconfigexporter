package exporter

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode"

	snapfmt "gitlab.com/easqrtech/iam/iscconfigexporter/snapshot"
)

// MappingAPI is independent of API: ordinary exporters need no mapping permissions.
// Implementations return the original SDK response bytes, not reserialized models.
type MappingAPI interface {
	GetMappings(context.Context, string) (MappingDownload, error)
}
type MappingDownload struct {
	Data      json.RawMessage
	APIOrigin string
}

// MappingRequest describes an explicit read from the target/current organization.
// ExportJobID is optional operator-supplied correlation, not a remote association.
type MappingRequest struct {
	SourceOrg    string
	TargetTenant string
	ExportJobID  string
}

func (r MappingRequest) Validate() error {
	if !portable(r.SourceOrg) || !portable(r.TargetTenant) {
		return errors.New("valid mapping source organization and target tenant are required")
	}
	if len(r.ExportJobID) > 256 || strings.IndexFunc(r.ExportJobID, unicode.IsControl) >= 0 {
		return errors.New("invalid mapping export job correlation")
	}
	return nil
}

// MappingProvenance binds saved bytes to their retrieval context. TargetTenant
// is operator-declared; APIOrigin records the actual SDK request origin.
type MappingProvenance struct {
	Schema       int       `json:"schema"`
	RetrievalID  string    `json:"retrieval_id"`
	SourceOrg    string    `json:"source_org"`
	TargetTenant string    `json:"target_tenant"`
	APIOrigin    string    `json:"api_origin"`
	APIVersion   string    `json:"api_version"`
	Endpoint     string    `json:"endpoint"`
	Retrieved    time.Time `json:"retrieved"`
	ExportJobID  string    `json:"export_job_id,omitempty"`
	Count        int       `json:"count"`
	SHA256       string    `json:"sha256"`
}
type MappingSnapshot struct {
	Mappings   json.RawMessage
	Provenance MappingProvenance
}

func mappingEndpoint(sourceOrg string) string {
	return "/v2026/configuration-hub/object-mappings/" + url.PathEscape(sourceOrg)
}
func mappingOrigin(origin string) bool {
	u, e := url.Parse(origin)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Path == ""
}

// ValidateMappings checks storage integrity without interpreting JSONPath or
// applying substitutions. Unknown mapping fields and original bytes survive.
func ValidateMappings(data []byte) (int, error) {
	v, e := snapfmt.DecodeJSON(data)
	if e != nil {
		return 0, errors.New("invalid mapping JSON")
	}
	items, ok := v.([]any)
	if !ok {
		return 0, errors.New("mapping response must be a JSON array")
	}
	seen := map[string]bool{}
	for _, v := range items {
		m, ok := v.(map[string]any)
		if !ok {
			return 0, errors.New("invalid mapping record")
		}
		for _, key := range []string{"objectMappingId", "objectType", "jsonPath", "sourceValue", "targetValue"} {
			s, ok := m[key].(string)
			if !ok || ((key == "objectMappingId" || key == "objectType" || key == "jsonPath") && s == "") {
				return 0, errors.New("missing or invalid mapping string field")
			}
		}
		if _, ok := m["enabled"].(bool); !ok {
			return 0, errors.New("mapping enabled status is required")
		}
		id := m["objectMappingId"].(string)
		if seen[id] {
			return 0, errors.New("duplicate mapping identity")
		}
		seen[id] = true
		for _, key := range []string{"created", "modified"} {
			if value, ok := m[key]; ok {
				if _, ok := value.(string); !ok {
					return 0, errors.New("invalid mapping timestamp field")
				}
			}
		}
	}
	return len(items), nil
}

// RetrieveMappings reads once through MappingAPI. No export job, backup action,
// tenant mutation or filesystem publication is performed here.
func RetrieveMappings(ctx context.Context, api MappingAPI, r MappingRequest) (MappingSnapshot, error) {
	var out MappingSnapshot
	if e := r.Validate(); e != nil {
		return out, e
	}
	if api == nil {
		return out, errors.New("mapping API is required")
	}
	if e := ctx.Err(); e != nil {
		return out, e
	}
	download, e := api.GetMappings(ctx, r.SourceOrg)
	if e != nil {
		return out, e
	}
	if e = ctx.Err(); e != nil {
		return out, e
	}
	data := append(json.RawMessage(nil), download.Data...)
	count, e := ValidateMappings(data)
	if e != nil {
		return out, e
	}
	origin := strings.TrimRight(download.APIOrigin, "/")
	if !mappingOrigin(origin) {
		return out, errors.New("invalid mapping API origin")
	}
	out.Mappings = data
	out.Provenance = MappingProvenance{Schema: 1, RetrievalID: token(), SourceOrg: r.SourceOrg, TargetTenant: r.TargetTenant, APIOrigin: origin, APIVersion: "v2026", Endpoint: mappingEndpoint(r.SourceOrg), Retrieved: time.Now().UTC(), ExportJobID: r.ExportJobID, Count: count, SHA256: snapfmt.Checksum(out.Mappings)}
	return out, nil
}
func validateMappingSnapshot(s MappingSnapshot) error {
	p := s.Provenance
	if e := (MappingRequest{SourceOrg: p.SourceOrg, TargetTenant: p.TargetTenant, ExportJobID: p.ExportJobID}).Validate(); e != nil {
		return e
	}
	count, e := ValidateMappings(s.Mappings)
	if e != nil {
		return e
	}
	if p.Schema != 1 || p.RetrievalID == "" || p.Retrieved.IsZero() || p.Count != count || p.SHA256 != snapfmt.Checksum(s.Mappings) || !mappingOrigin(p.APIOrigin) || p.APIVersion != "v2026" || p.Endpoint != mappingEndpoint(p.SourceOrg) {
		return errors.New("inconsistent mapping provenance")
	}
	return nil
}
