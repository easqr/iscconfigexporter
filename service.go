package exporter

import (
	"context"
	"errors"
	"fmt"
	snapfmt "github.com/easqr/iscconfigexporter/snapshot"
	"log/slog"
	"time"
)

// Service runs a complete export with required Engine and Store dependencies.
// Lifecycle and Logger are optional. Callers must not change its dependencies or
// policy during a run; concurrent use requires concurrency-safe dependencies.
type Service struct {
	// Engine creates and downloads the remote export using its policy.
	Engine Engine
	// Store saves the validated snapshot.
	Store Store
	// Lifecycle evaluates backups after publication. Nil skips evaluation and
	// reports a limitation if the policy enables it.
	Lifecycle BackupLifecycle
	// Logger receives a publication summary; nil disables this logging.
	Logger *slog.Logger
}

// Export creates a remote job, builds a snapshot, saves it and evaluates backups.
// An export or publication failure is returned as error; [Result.Manifest] may be
// nonzero if publication became visible before a later storage error. Such an
// error requires inspecting the stored snapshot before retrying.
// Failures with a known remote job ID wrap their cause in [ExportError], including
// polling, download, preparation and publication failures. No manifest is invented
// for a job that did not produce a published snapshot.
//
// Backup evaluation errors do not undo a saved export and do not become the
// returned error: they appear in [Result.CleanupError], with any partial decisions
// in Result.Cleanup. A nonempty Cleanup.Limitation alone is not a failure.
// The policy timeout and ctx bound the workflow except for provider-specific work,
// such as the SDK's separate native token acquisition.
func (s Service) Export(ctx context.Context) (Result, error) {
	var out Result
	if s.Store == nil {
		return out, errors.New("artifact store is required")
	}
	timeout, e := ParseDuration(s.Engine.Config.Timeout)
	if e != nil {
		return out, e
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	d, e := s.Engine.ExportAndDownload(ctx)
	if e != nil {
		return out, exportFailure(d.JobID, e)
	}
	snap, e := BuildSnapshotWithFilenameOptions(s.Engine.Config.Tenant, d, time.Now(), snapfmt.FilenameOptions{Mode: s.Engine.Config.FilenameMode, Escape: s.Engine.Config.FilenameEscape, AppendID: s.Engine.Config.FilenameAppendID})
	if e != nil {
		return out, exportFailure(d.JobID, fmt.Errorf("cannot prepare export files: %w", e))
	}
	out.Manifest, e = s.Store.Publish(ctx, snap)
	if e != nil {
		return out, exportFailure(d.JobID, fmt.Errorf("cannot save export snapshot: %w", e))
	}
	if s.Logger != nil {
		s.Logger.InfoContext(ctx, "export snapshot saved", "objects", len(snap.Artifacts))
	}
	if s.Lifecycle != nil {
		out.Cleanup, e = s.Lifecycle.Cleanup(ctx, s.Engine.Config.Tenant, d.JobID, s.Engine.Config.Backup)
		if e != nil {
			out.CleanupError = "backup evaluation failed after the export was saved; check Configuration Hub read access and the ownership registry"
		}
	} else if s.Engine.Config.Backup.Enabled {
		out.Cleanup.Limitation = "backup evaluation is enabled but no backup integration is configured; no backups were deleted"
	}
	return out, nil
}

var _ Exporter = Service{}
