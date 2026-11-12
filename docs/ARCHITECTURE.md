# Architecture

Use this reference to understand the library's components and storage guarantees. Start
with the [Go library guide](LIBRARY.md) for runnable examples. Contributor checks and
test coverage are in [development](DEVELOPMENT.md).

## Components

| Component | Responsibility |
| --- | --- |
| Root `exporter` package | Export settings (`Config`), retrieval (`Engine`, `ExportAPI`), storage (`Store`), and backup interfaces (`BackupLifecycle`, `Registry`, `BackupAPI`) |
| `snapshot` package | `Artifact`, `Changes`, `Manifest`, and `Snapshot` types, plus decoding and readers |
| `sdk` package | Official SailPoint SDK integration; `NewWithConfiguration` accepts an SDK configuration |
| CLI | Loads settings and connects the library components to run the selected action |

SDK models stay inside the `sdk` package. Applications can choose their own retrieval
and storage implementations through the library interfaces.

An export follows these steps:

1. Discover exportable types and validate the requested selection.
2. Start one export job and poll until it reports `COMPLETE`.
3. Download the result, validate every object envelope, and build the snapshot.
4. Save the snapshot and update the current-generation pointer.
5. Evaluate backups if enabled.

Export success and backup evaluation success are reported separately.

`Engine.ExportAndDownload` retains any known remote job ID in its returned download,
including on failure. `Service.Export` wraps failures with a known ID in `*ExportError`,
preserving the underlying error chain and any actual publication metadata. CLI
diagnostics retain this ID on cancellation and timeout; a local interruption does not
cancel the remote job.

## Snapshot construction

`BuildSnapshot` validates downloaded bytes and builds the snapshot in memory, without
filesystem or network access. It requires a matching tenant, selected types, nonempty
stable identity, envelope version `1`, and an object payload. A payload `object.id`,
when present, must match `self.id`. Building, publication and reading share envelope
validation. Unknown fields and signatures are retained.

JSON keys are sorted using `encoding/json`; arrays keep source order and numbers use
`json.Number` to avoid floating-point precision loss. Duplicate JSON keys are rejected.
Exporter-generated completion timestamps are stored in the manifest; timestamps present
in exported payloads are preserved. This is deterministic JSON, not a cryptographic JSON
canonicalization standard; no signatures are regenerated or claimed verified.

By default, IDs become `id-<UTF-8 hex>.json`, a reversible portable encoding that avoids colon
substitution and case-fold collisions. Slash, backslash, control characters, dot
identities and IDs over 100 bytes fail validation. Tenant/type names use restricted
portable identifiers; device names are forbidden. Case-folded path collisions fail
closed.

### Why filenames default to encoded IDs

ISC object names are display values, not portable filesystem identifiers. Naming
constraints vary by object type; the published Source and Transform schemas do not
provide a filename-safe character whitelist. A reported role export failure involving
`:` and backslashes demonstrates that this is a real compatibility issue, not only a
hypothetical one. See SailPoint's [Source schema](https://raw.githubusercontent.com/sailpoint-oss/api-specs/main/idn/v3/schemas/Source.yaml),
[Transform schema](https://raw.githubusercontent.com/sailpoint-oss/api-specs/main/idn/v3/schemas/Transform.yaml)
and the [reported export failure](https://github.com/yannick-beot-sp/vscode-sailpoint-identitynow/issues/108).

Using an object name unchanged as `<name>.json` creates several risks:

- Linux and macOS interpret `/` as a path separator; NUL cannot be filename content.
- Windows additionally reserves backslash, colon, asterisk, question mark, double
  quote, angle brackets, pipe and control characters. Device names such as `CON` and
  `NUL` remain reserved even with a `.json` extension.
- Default macOS and Windows filesystems can collapse names that differ only by case;
  macOS APFS also treats canonically equivalent Unicode spellings as the same name.
- Object-name character limits do not guarantee filesystem byte limits or full-path
  limits. For example, ext4 limits a filename to 255 bytes, including the extension.
- Replacing invalid characters can map different names to the same filename, and an
  object rename would change its path despite retaining the same identity.

Ordinary spaces and punctuation such as parentheses and ampersands are valid filename
content on all three platforms; they do not by themselves justify replacement. The
decision is based on the complete set of portability and identity requirements. See
the [Linux pathname rules](https://man7.org/linux/man-pages/man7/pathname.7.html),
[ext4 filename limits](https://docs.kernel.org/filesystems/ext4/directory.html),
[Windows naming rules](https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file),
and [Apple's APFS normalization documentation](https://developer.apple.com/library/archive/documentation/FileManagement/Conceptual/APFS_Guide/FAQ/FAQ.html).

Encoding the stable ID in lowercase hexadecimal preserves distinct accepted IDs,
keeps paths unchanged when objects are renamed, and avoids name sanitization and
Unicode normalization collisions. The 100-byte ID limit caps the filename at 208
ASCII bytes, including `id-` and `.json`. The original name remains readable in the
object JSON. Callers can explicitly select ID or name--ID filenames instead;
these modes fail before publication if portability checks find an unresolved issue.
The opt-in `filename_escape` policy reversibly percent-encodes unsafe characters
and literal percent signs. Name mode adds IDs to all files in a collision group after escaping and profile
prefixes; `filename_append_id` forces suffixes on every name-mode file and defaults
to false. Generated suffixes are checked again for secondary collisions.
Strict ID exports use schema 2, escaped ID exports use schema 3, and new name--ID
exports use schema 5, with explicit naming metadata. Lifecycle-state
filenames prepend the parent name from `object.identityProfileRef`; incomplete
references fail name-mode validation. The prefix is escaped and length-checked
with the state name and ID. Schema 3 retains the previous unprefixed name--ID
interpretation and escaped ID exports. Readers retain schema
2's earlier name-only interpretation and schema 4's always-suffixed profile-aware
filenames. Default hex exports retain schema 1. Readers
validate paths against the declared mode and object envelope.
A check-only action downloads an export and reports both alternatives without
publishing files or evaluating backups. See [filename compatibility](SNAPSHOTS.md#filename-compatibility)
for conservative comparison rules and limits. Portable artifact filenames do not imply Windows publication support:
`FileStore` has separate POSIX storage requirements described below.

The [snapshot reference](SNAPSHOTS.md) describes filenames, manifest fields and reader
modes. Go consumers use `Manifest.Artifacts` (relative paths and SHA-256 checksums),
`Counts`, `Types`, `JobID`, `Completed` and `Changes`. Artifacts contain the complete
original object envelope; top-level download metadata remains operational input, not
individual object content. ExportAndDownload and BuildSnapshot may be used independently
of filesystem publication.

## Filesystem publication

`FileStore` stages a complete private generation under
`.isc-snapshots/<tenant>-<random>`. It writes and syncs files, directories and the
generation parent before atomically renaming a relative symlink at `<output>/<tenant>`.

A directory lock allows one cooperating publisher per tenant. A crash leaves the
previous pointer or a fully staged new generation, plus possibly an orphan generation,
temporary pointer or stale lock. Inspect the pointer and manifest before manually
removing a stale lock; never remove an active writer's lock. Unreferenced generations
remain for recovery and are not automatically removed.

Individual artifacts use permissions `0600` and directories `0700`. Unchanged artifacts
are hardlinked, retaining inode/mtime. Treat generations as read-only: editing a
hardlinked artifact also edits older generations; checksum validation will block the
next publication.

Only files listed in the previous manifest are reconciled. Unmanaged regular files are
copied; symlinks, special files and collisions fail publication. A filtered export
replaces the managed snapshot with exactly the selected types; excluded types from an
earlier snapshot appear in Deleted. Previous generations remain on disk. An existing
ordinary tenant directory is never adopted or replaced. The reserved manifest.json
belongs exclusively to the exporter.

The output root must be private, operator-controlled storage; `os.Root` contains
operations beneath it. It must not be concurrently modified by unrelated processes.
Generation directories and nested files must not contain symlinks. Use a local POSIX
filesystem with atomic symlink rename, directory fsync and hardlink support. Windows,
network filesystems and hostile shared roots are unsupported.

The pointer rename makes a complete snapshot visible in one step. Readers needing
consistency across several files must resolve a generation using `CurrentPath`, then
read its manifest and artifacts from that returned path. Re-resolving the tenant symlink
for each read can mix generations.

Cancellation is checked immediately before the pointer replacement; cancellation racing
with rename may still publish. Failure after rename during parent fsync reports an error
with a visible complete snapshot whose crash durability is uncertain. The implementation
provides atomic visibility plus explicit syncs, not a guarantee against faulty storage
hardware. Filesystem and context failures before rename preserve the previous
generation.

## Backup lifecycle

`BackupManager` evaluates all candidates before deletion in a custom integration. Backup
evaluation is disabled by default; the supplied SDK integration never deletes.

Age uses a strict older-than boundary; `d` is a fixed 24-hour duration. Keep-latest
protects all equally newest successful verified backups, even those without deletion
permission. Incomplete, failed, partial, unknown, duplicated and metadata-mismatched
records are skipped. A live recheck must match enumeration exactly, and a protected
successor must still exist. Audit decisions distinguish `retain`, `skip`,
`would_delete`, `deleted`, and `delete_failed`. No raw API error details are surfaced.
Service calls lifecycle only after publication and reports cleanup failure
independently.

The SDK-backed lifecycle is intentionally non-destructive because reliable
SP-Config/Configuration Hub ownership and conditional deletion semantics are
undocumented.

Registry persistence uses an exclusive directory lock, staged file, syncs, atomic
rename, restricted permissions, duplicate rejection, and explicit tenant filtering. It
is a trusted local ledger, not a remote authority. No records are auto-created based on
names or inferred ID equality. See [API capabilities and limits](API.md) for ownership
and deletion requirements.

## SDK integration

Connection settings are loaded from exact local `config.json` paths or `SAIL_*`.
Authentication is delegated to the SailPoint SDK. One root client supplies Beta and
V2026 services, with native token caches for each version. The SDK's retryable HTTP
client supplies bounded read retries, backoff and Retry-After handling. Engine handles
business-level job polling, not HTTP retries. Adapter gating prevents concurrent
mutation of SDK token caches; context cancellation can interrupt gate waits/API
requests, but cannot interrupt the SDK's separate native token acquisition. See
[configuration](CONFIGURATION.md) for operational limitations and the [SDK integration
review](development/SDK_INTEGRATION.md) for implementation details.

## Independent mapping retrieval

`MappingAPI`, `MappingRequest`, `MappingSnapshot` and `RetrieveMappings` provide a
separate library path for read-only Object Mapping retrieval. They are separate from the
configuration-export API. `sdk.Adapter.GetMappings` calls the existing V2026
Configuration Hub client under the same gate and HTTP retry policy; SDK-generated
per-version token caches remain native. Raw mapping bytes survive the SDK
response-decoding boundary, and storage validation rejects ambiguous or malformed
mapping records without interpreting JSONPath.

`MappingFileStore` stages and publishes `mappings.json` plus checksum-bound
`provenance.json` into a new independent directory, with restricted permissions,
cooperating-writer locking, syncs and atomic rename. Existing bundles are refused. The
CLI enforces separation from the configured configuration export tree before
authentication and again before publication. No mapping data is inserted into object
envelopes or snapshot manifests. The explicit CLI retrieval branch bypasses
export/backup/cleanup orchestration. See [MAPPINGS.md](MAPPINGS.md) for the contract and
offline differ boundary.
