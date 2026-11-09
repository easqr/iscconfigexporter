// Package exporter exports SailPoint ISC configuration to local snapshots.
//
// Start with [DefaultConfig], set the tenant and output directory, and supply an
// ExportAPI and a [Store] to Service. The sdk subpackage supplies a SailPoint SDK
// adapter; [FileStore] saves complete snapshots. [Service.Export] performs remote
// export, validation and publication, followed by an optional BackupLifecycle.
// Inspect both its returned error and Result.CleanupError.
// Service failures with a known remote job ID wrap the cause in [ExportError];
// retrieve it with errors.As rather than parsing diagnostic text.
//
// For separate workflow stages, [Engine.ExportAndDownload] creates and downloads
// a remote export job, and [BuildSnapshot] validates and organizes its records
// without writing files. Snapshot models and SDK-free readers live in the
// gitlab.com/easqrtech/iam/iscconfigexporter/snapshot package.
//
// SDK connection settings are separate from [Config], which contains export policy
// only. The supplied SDK backup integration provides read-only inventory; custom
// deletion integrations require independent ownership and eligibility guarantees.
package exporter
