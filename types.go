// Package exporter exports ISC configuration into portable, deterministic snapshots.
package exporter

import (
	"context"
	"encoding/json"
	snapfmt "gitlab.com/easqrtech/iam/iscconfigexporter/snapshot"
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
	Publish(context.Context, snapfmt.Snapshot) (snapfmt.Manifest, error)
}
type Lifecycle interface {
	Cleanup(context.Context, string, string, BackupConfig) (CleanupReport, error)
}
type Result struct {
	Manifest     snapfmt.Manifest `json:"manifest"`
	Cleanup      CleanupReport    `json:"cleanup"`
	CleanupError string           `json:"cleanup_error,omitempty"`
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
