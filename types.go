package exporter

import (
	"context"
	"encoding/json"
	"time"

	snapfmt "gitlab.com/easqrtech/iam/iscconfigexporter/snapshot"
)

// ExportAPI provides the remote operations used by Engine.ExportAndDownload.
// Implementations must preserve complete export data, including unknown fields.
// Engine supplies a deadline; providers must document authentication or other work
// that cannot honor it, as well as their concurrency guarantees.
type ExportAPI interface {
	// Discover returns the tenant's exportable type names with their exact casing.
	Discover(ctx context.Context) ([]string, error)
	// Start creates one export job for types and returns its nonempty job ID.
	// An ambiguous failure may have created a job; creation must not be retried blindly.
	// If a provider knows the job ID despite an error, it should return both.
	Start(ctx context.Context, types []string) (string, error)
	// Status returns NOT_STARTED, IN_PROGRESS, COMPLETE, CANCELLED or FAILED.
	// Engine rejects unrecognized states and downloads only after COMPLETE.
	Status(ctx context.Context, jobID string) (string, error)
	// Download returns the full JSON document for a completed job, including the
	// tenant and objects array. It must not strip unknown envelope or payload fields.
	Download(ctx context.Context, jobID string) (json.RawMessage, error)
}

// Exporter runs an export and returns its publication and backup evaluation results.
// A nil error does not imply that optional backup evaluation succeeded: inspect
// Result.CleanupError as well. Service implements this interface.
type Exporter interface {
	// Export runs the workflow. Inspect both the returned error and Result.CleanupError;
	// publication metadata may be nonzero on an error after output becomes visible.
	// Service failures with a known remote job ID can be inspected as *ExportError.
	Export(ctx context.Context) (Result, error)
}

// Store saves complete snapshots and returns their publication metadata.
type Store interface {
	// Publish stores snapshot and returns its manifest, including computed changes.
	// Implementations must document their commit point and error behavior. FileStore
	// can return a nonzero manifest and an error when output is already visible.
	Publish(ctx context.Context, snapshot snapfmt.Snapshot) (snapfmt.Manifest, error)
}

// BackupLifecycle performs optional backup evaluation after snapshot publication.
// Custom implementations can delete backups only when ownership and safe deletion
// eligibility are independently established. The SDK integration is read-only.
type BackupLifecycle interface {
	// Cleanup honors policy.Enabled and policy.Policy and returns audit decisions.
	// currentExportJobID is the export job ID, not proof of a Configuration Hub
	// backup identity. Partial decisions can accompany an error. Service records
	// that error as a sanitized Result.CleanupError without undoing the export.
	Cleanup(ctx context.Context, tenant, currentExportJobID string, policy BackupConfig) (CleanupReport, error)
}

// Result describes snapshot publication and the subsequent backup evaluation.
type Result struct {
	// Manifest describes the saved snapshot. It can be nonzero alongside an export
	// error if publication became visible before a directory-sync failure.
	Manifest snapfmt.Manifest `json:"manifest"`
	// Cleanup records optional backup decisions and ownership limitations.
	Cleanup CleanupReport `json:"cleanup"`
	// CleanupError is a sanitized message, not an error value or an errors.Is target.
	// It is nonempty when backup evaluation failed after successful publication;
	// Service.Export still returns a nil error in that case.
	CleanupError string `json:"cleanup_error,omitempty"`
}

// CleanupReport records backup decisions independently of export success.
type CleanupReport struct {
	// Decisions contains one audit entry per evaluated decision. It can be nil
	// when evaluation is disabled or there are no records to report.
	Decisions []Decision `json:"decisions"`
	// Limitation explains why evaluation cannot establish deletion eligibility.
	// A limitation is not itself an export or cleanup error.
	Limitation string `json:"limitation,omitempty"`
}

// Decision explains what backup evaluation did with a backup record.
type Decision struct {
	// ID is the remote backup ID, which is not necessarily an export job ID.
	ID string `json:"id"`
	// Action is retain, skip, would_delete, deleted or delete_failed.
	Action string `json:"action"`
	// Reason is a human-readable explanation; callers should not parse its wording.
	Reason string `json:"reason"`
}

// Backup is an inventory record supplied by a BackupAPI implementation.
// Missing or uncertain eligibility information must not authorize deletion.
type Backup struct {
	// CanDelete reports explicit remote permission at enumeration time; it is not
	// an ownership guarantee or permission to skip a later eligibility recheck.
	CanDelete bool
	// Partial is true for partial or uncertain backup completeness.
	Partial bool
	// ID identifies the remote backup, independently of any export job ID.
	ID string
	// Tenant identifies the tenant that owns the backup.
	Tenant string
	// Status must be COMPLETE for a record to be considered fully successful.
	Status string
	// Provenance is provider-defined creation evidence for a custom verifier.
	// The supplied SDK inventory does not populate it.
	Provenance string
	// Created is the remote creation time. A zero time prevents deletion.
	Created time.Time
}

// Ownership records a claimed association supported by backup creation evidence.
// A record alone is insufficient: OwnershipVerifier must verify it independently.
type Ownership struct {
	// Tenant is the tenant associated with the recorded backup.
	Tenant string
	// OperationID identifies the creation operation supported by the evidence.
	// It must not be inferred from a matching backup ID.
	OperationID string
	// BackupID is the remote backup ID established by the creation evidence.
	BackupID string
	// Provenance is a provider-defined reference to evidence interpreted by the verifier.
	Provenance string
	// Created must match the remote backup's creation time for deletion eligibility.
	Created time.Time
}

// Registry stores local backup creation evidence; it does not prove remote ownership.
type Registry interface {
	// Records returns ownership records for tenant. No records is a successful result.
	Records(ctx context.Context, tenant string) ([]Ownership, error)
	// Register records creation evidence and must reject duplicate tenant/backup IDs.
	Register(ctx context.Context, ownership Ownership) error
}

// BackupAPI supplies backup inventory and, for verified custom integrations, deletion.
// Deletion requires independent ownership verification and safe remote eligibility.
type BackupAPI interface {
	// List returns backup records for tenant, without inferring ownership.
	List(ctx context.Context, tenant string) ([]Backup, error)
	// Get returns exactly one matching backup or an error if it is missing or ambiguous.
	Get(ctx context.Context, tenant, backupID string) (Backup, error)
	// Delete removes an eligible backup. The SDK implementation always returns
	// ErrOwnershipUnavailable. Custom providers must enforce safe deletion conditions.
	Delete(ctx context.Context, tenant, backupID string) error
}

// OwnershipVerifier checks current remote evidence against a local ownership claim.
// Implementations must not simply trust the registry or equality of unrelated IDs.
type OwnershipVerifier interface {
	// Verify returns true only when backup currently matches independent creation
	// evidence in ownership. False or an error prevents deletion.
	Verify(ctx context.Context, backup Backup, ownership Ownership) (bool, error)
}
