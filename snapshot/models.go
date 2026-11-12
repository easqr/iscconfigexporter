package snapshot

import "time"

// Artifact describes an object file relative to its snapshot generation.
// Data carries bytes for publication; it is omitted from serialized manifests.
type Artifact struct {
	// Path is the canonical relative path under the selected filename mode.
	Path string `json:"path"`
	// SHA256 is the lowercase hexadecimal SHA-256 checksum of the exact file bytes.
	SHA256 string `json:"sha256"`
	// Data contains the full object envelope when preparing a snapshot for storage.
	// A manifest decoded from disk contains metadata only and leaves Data nil.
	Data []byte `json:"-"`
}

// Changes lists managed file paths relative to the previously published snapshot.
// The store computes these lists during publication; empty categories may be nil.
type Changes struct {
	// Created lists newly managed paths.
	Created []string `json:"created"`
	// Modified lists prior paths whose file bytes changed.
	Modified []string `json:"modified"`
	// Unchanged lists paths with the same bytes as before.
	Unchanged []string `json:"unchanged"`
	// Deleted lists prior managed paths absent from the new selection or export.
	Deleted []string `json:"deleted"`
}

// Manifest describes an exported generation using schemas 1-5 (hex, literal, escaped/name+ID, profile-prefixed names, conditional IDs). It is not a signature
// or proof of remote job success. In TreeMode, its counts and artifact checksums
// describe the source export, not the edited inventory in Input.Objects.
type Manifest struct {
	// Schema is the exporter storage format version: 1 for hex, 2 for legacy literal naming, 3 for escaping/name+ID, 4 for always-suffixed profile names, 5 for conditional IDs.
	Schema int `json:"schema"`
	// FilenameMode is required for schemas 2-5; omitted for legacy schema 1 (hex).
	FilenameMode FilenameMode `json:"filename_mode,omitempty"`
	// FilenameEscape enables canonical percent escaping in schemas 3-5.
	FilenameEscape bool `json:"filename_escape,omitempty"`
	// FilenameAppendID forces IDs on all schema 5 name-mode files.
	FilenameAppendID bool `json:"filename_append_id,omitempty"`
	// Tenant is the tenant identity checked against the export document.
	Tenant string `json:"tenant"`
	// JobID identifies the remote export job, not a verified backup association.
	JobID string `json:"job_id"`
	// Completed is the exporter-generated completion time, not an ISC object timestamp.
	Completed time.Time `json:"completed"`
	// Types is the selected scope, including types with zero objects.
	Types []string `json:"types"`
	// Counts maps every selected type to its nonnegative exported object count.
	Counts map[string]int `json:"counts"`
	// Artifacts lists object-file metadata. In a freshly built snapshot its Data
	// fields can also be populated, but JSON serialization omits those bytes.
	Artifacts []Artifact `json:"artifacts"`
	// Changes describes publication relative to the previous current generation.
	Changes Changes `json:"changes"`
}

// Snapshot is an in-memory export ready for publication by the root exporter package.
// Build it with exporter.BuildSnapshot. The root FileStore validates consistency
// between Manifest.Artifacts and Artifacts before writing any generation files.
type Snapshot struct {
	// Manifest contains matching artifact metadata, counts and export context.
	Manifest Manifest
	// Artifacts contains the complete files to publish, including Data. The same
	// records must appear in Manifest.Artifacts with matching paths, checksums and data.
	Artifacts []Artifact
}
