// Package snapshot reads and validates ISC exporter snapshots without the SailPoint SDK.
// It also provides the shared manifest, artifact and in-memory snapshot models.
//
// Use [Read] for disk files or [FromFiles] for in-memory bytes. [SnapshotMode] checks an
// untouched export against its manifest; [TreeMode] inventories an intentionally
// edited working copy and can operate without a manifest when [Options.Types] is
// supplied. Neither reader performs remote requests or verifies authenticity.
//
// Input is a validated in-memory view. Keep its data and construction options
// unchanged; only [Label] and [Environment] are intended to be relabeled. [Clone]
// returns detached data after revalidation. Readers require an explicit mode,
// canonical filenames and envelope version 1, and preserve unknown object fields.
package snapshot
