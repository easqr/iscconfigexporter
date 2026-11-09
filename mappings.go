package exporter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"

	snapfmt "github.com/easqr/iscconfigexporter/snapshot"
)

// MappingAPI retrieves mapping responses from the authenticated target tenant.
// It is separate from ExportAPI, so ordinary exports need no mapping permissions.
type MappingAPI interface {
	// GetMappings returns original response bytes and their actual HTTPS API origin.
	// sourceOrg selects the source organization; default selects unassociated mappings.
	// Implementations must preserve disabled records and unknown fields, and document
	// any authentication work that cannot honor ctx.
	GetMappings(ctx context.Context, sourceOrg string) (MappingDownload, error)
}

// MappingDownload is a raw mapping response and its request origin, before archive
// provenance is added. It contains tenant configuration values and can be sensitive.
type MappingDownload struct {
	// Data is the original JSON response array, not reserialized SDK models.
	Data json.RawMessage
	// APIOrigin is scheme://host[:port] for the actual request, without credentials,
	// path or query. It is distinct from the operator-declared target tenant.
	APIOrigin string
}

// MappingRequest selects mappings from the authenticated target organization.
// TargetTenant is declared context, not a change to the adapter's SDK connection.
type MappingRequest struct {
	// SourceOrg is a source organization identifier, or default for unassociated mappings.
	SourceOrg string
	// TargetTenant labels the target tenant in saved provenance. It is not remotely verified.
	TargetTenant string
	// ExportJobID is optional operator-supplied correlation, not a verified association.
	// It must be at most 256 bytes and contain no control characters.
	ExportJobID string
}

// Validate checks identifiers and correlation syntax locally, without verifying
// permissions, tenant identity, source existence or the recorded export job.
func (r MappingRequest) Validate() error {
	if !portable(r.SourceOrg) || !portable(r.TargetTenant) {
		return errors.New("mapping source organization and target tenant must be valid tenant identifiers; see docs/MAPPINGS.md")
	}
	if len(r.ExportJobID) > 256 || strings.IndexFunc(r.ExportJobID, unicode.IsControl) >= 0 {
		return errors.New("the recorded export job ID must be at most 256 bytes with no control characters")
	}
	return nil
}

// MappingProvenance records archive integrity and retrieval context. Its checksum
// detects byte changes; the metadata does not authenticate the remote source or
// prove that mappings and a configuration export were captured together.
type MappingProvenance struct {
	// Schema is the archive metadata format version, currently 1.
	Schema int `json:"schema"`
	// RetrievalID is a random local retrieval identifier, not a remote job ID.
	RetrievalID string `json:"retrieval_id"`
	// SourceOrg is the requested organization or default source group.
	SourceOrg string `json:"source_org"`
	// TargetTenant is operator-declared and not remotely verified.
	TargetTenant string `json:"target_tenant"`
	// APIOrigin records the actual HTTPS request origin without credentials or query.
	APIOrigin string `json:"api_origin"`
	// APIVersion identifies the SDK endpoint version, currently v2026.
	APIVersion string `json:"api_version"`
	// Endpoint is the mapping request path for SourceOrg.
	Endpoint string `json:"endpoint"`
	// Retrieved is the local retrieval-completion timestamp in UTC.
	Retrieved time.Time `json:"retrieved"`
	// ExportJobID is optional unverified operator-supplied export correlation.
	ExportJobID string `json:"export_job_id,omitempty"`
	// Count includes every returned record, including disabled mappings.
	Count int `json:"count"`
	// SHA256 is the lowercase hexadecimal checksum of exact mappings.json bytes.
	SHA256 string `json:"sha256"`
}

// MappingSnapshot is a retrieved mapping array with checksum-bound provenance.
// It is independent of an object snapshot and does not contain manifest.json.
type MappingSnapshot struct {
	// Mappings is the original response array, including disabled and unknown fields.
	Mappings json.RawMessage
	// Provenance describes and checksums Mappings. Keep both unchanged before publication.
	Provenance MappingProvenance
}

func mappingEndpoint(sourceOrg string) string {
	return "/v2026/configuration-hub/object-mappings/" + url.PathEscape(sourceOrg)
}
func mappingOrigin(origin string) bool {
	u, e := url.Parse(origin)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Path == ""
}

// ValidateMappings checks the response-array schema and returns its total record
// count, including disabled mappings. It rejects malformed or duplicate keys and
// identities, but does not verify archive provenance or a checksum, interpret
// JSONPath, or apply substitutions. Unknown fields are accepted; data is not changed.
func ValidateMappings(data []byte) (int, error) {
	v, e := snapfmt.DecodeJSON(data)
	if e != nil {
		return 0, errors.New("mapping data contains invalid JSON or duplicate JSON keys")
	}
	items, ok := v.([]any)
	if !ok {
		return 0, errors.New("mapping response must be a JSON array")
	}
	seen := map[string]bool{}
	for _, v := range items {
		m, ok := v.(map[string]any)
		if !ok {
			return 0, errors.New("each mapping record must be a JSON object")
		}
		for _, key := range []string{"objectMappingId", "objectType", "jsonPath", "sourceValue", "targetValue"} {
			s, ok := m[key].(string)
			if !ok || ((key == "objectMappingId" || key == "objectType" || key == "jsonPath") && s == "") {
				return 0, fmt.Errorf("mapping field %q must be a string (and nonempty for identity, type and path fields)", key)
			}
		}
		if _, ok := m["enabled"].(bool); !ok {
			return 0, errors.New("each mapping record must include a boolean enabled field")
		}
		id := m["objectMappingId"].(string)
		if seen[id] {
			return 0, errors.New("mapping data contains duplicate objectMappingId values")
		}
		seen[id] = true
		for _, key := range []string{"created", "modified"} {
			if value, ok := m[key]; ok {
				if _, ok := value.(string); !ok {
					return 0, fmt.Errorf("mapping field %q must be a string when present", key)
				}
			}
		}
	}
	return len(items), nil
}

// RetrieveMappings fetches mappings for the requested source group, copies the
// response bytes, validates them and adds local retrieval provenance and a checksum.
// The API determines the authenticated target; TargetTenant is a declared label.
// It returns data in memory without writing files or starting an export or backup.
// Use MappingFileStore.Publish to save it. Provider authentication can have separate
// cancellation and timeout limits from ctx.
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
