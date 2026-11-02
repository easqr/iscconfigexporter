// Package exporter exports ISC configuration into portable, deterministic snapshots.
package exporter

import (
	"context"
	"encoding/json"
	"time"
)

type API interface {
	Discover(context.Context) ([]string, error)
	Start(context.Context, []string) (string, error)
	Status(context.Context, string) (string, error)
	Download(context.Context, string) (json.RawMessage, error)
}
type Exporter interface {
	Export(context.Context) (Result, error)
}
type Store interface {
	Publish(context.Context, Snapshot) (Manifest, error)
}
type Lifecycle interface {
	Cleanup(context.Context, string, string, BackupConfig) (CleanupReport, error)
}
type Result struct {
	Manifest     Manifest      `json:"manifest"`
	Cleanup      CleanupReport `json:"cleanup"`
	CleanupError string        `json:"cleanup_error,omitempty"`
}
type Artifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Data   []byte `json:"-"`
}
type Changes struct {
	Created   []string `json:"created"`
	Modified  []string `json:"modified"`
	Unchanged []string `json:"unchanged"`
	Deleted   []string `json:"deleted"`
}
type Manifest struct {
	Schema    int            `json:"schema"`
	Tenant    string         `json:"tenant"`
	JobID     string         `json:"job_id"`
	Completed time.Time      `json:"completed"`
	Types     []string       `json:"types"`
	Counts    map[string]int `json:"counts"`
	Artifacts []Artifact     `json:"artifacts"`
	Changes   Changes        `json:"changes"`
}
type Snapshot struct {
	Manifest  Manifest
	Artifacts []Artifact
}
type CleanupReport struct {
	Decisions  []Decision `json:"decisions"`
	Limitation string     `json:"limitation,omitempty"`
}
type Decision struct {
	ID     string `json:"id"`
	Action string `json:"action"`
	Reason string `json:"reason"`
}
type Backup struct {
	CanDelete bool
	Partial   bool

	ID, Tenant, Status, Provenance string
	Created                        time.Time
}
type Ownership struct {
	Tenant, OperationID, BackupID, Provenance string
	Created                                   time.Time
}
type Registry interface {
	Records(context.Context, string) ([]Ownership, error)
	Register(context.Context, Ownership) error
}
type BackupAPI interface {
	List(context.Context, string) ([]Backup, error)
	Get(context.Context, string, string) (Backup, error)
	Delete(context.Context, string, string) error
}

// OwnershipVerifier must provide independent, current evidence; a registry alone is insufficient.
type OwnershipVerifier interface {
	Verify(context.Context, Backup, Ownership) (bool, error)
}
