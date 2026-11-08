# Execution and recovery

## Export lifecycle

An export validates the policy, discovers and selects supported types, starts one job, polls for completion, downloads and validates the objects, then publishes a complete snapshot. Optional backup evaluation runs after publication.

The exporter waits on `NOT_STARTED` and `IN_PROGRESS`, downloads on `COMPLETE`, and stops on `FAILED`, `CANCELLED`, or an unrecognized status. Malformed envelopes, a missing objects array, tenant mismatches and duplicate identities fail the export. A valid empty objects array publishes an empty managed snapshot and removes previously managed paths from the current view. Selecting no exportable types is an error before job creation.

Only transient read responses are retried. Export initiation is never retried automatically. If initiation fails ambiguously, inspect ISC before repeating: a remote job may already exist. Cancellation does not cancel the remote job, which may continue running. See [configuration](CONFIGURATION.md) for retry settings and authentication timeout limits.

## Results and errors

Use `isc-exporter -help` for action examples and options. Invalid command-line options produce one error with usage guidance, rather than a full help listing. CLI failures use an `error:` prefix and a nonzero exit status.

A successful export writes a JSON result to standard output. Structured logs and failures go to standard error. SDK token transport failures may also print to standard output; account for this when processing results in scripts.

The result contains:

| JSON field | Meaning |
| --- | --- |
| `manifest` | Metadata for the published snapshot, including artifact checksums and changed paths |
| `cleanup` | Backup evaluation decisions and any ownership limitation |
| `cleanup_error` | Cleanup failed after the export was published; present only on failure |

The CLI emits the published result before returning a nonzero exit status for cleanup failure. An ownership limitation in `cleanup.limitation` is not itself an export failure. The supplied SDK adapter cannot delete backups, regardless of the retention policy. See [API capabilities and limits](API.md).

## Snapshot storage

Snapshots use immutable generation directories and a tenant symlink pointing to the current generation. Resolve that pointer once when reading several files; Go applications can use `CurrentPath`. Resolving it for each file can mix generations during an export.

Treat generation files as read-only. Unchanged artifacts can be hardlinked across generations, so editing one can alter older snapshots too. Changed or missing managed files prevent the next publication.

Only paths listed in the previous manifest are reconciled. Unmanaged regular files in the current generation are copied forward; unmanaged files elsewhere in the output root are untouched. Name collisions, nested symlinks, special files, invalid manifests, and an ordinary directory at the tenant pointer path block publication. Preserve any local edits before resolving a conflict or choosing a fresh output root.

Use private, trusted local POSIX storage with atomic rename, hardlinks and directory sync support. Capacity, quota and permission failures can prevent publication. Old and orphan generations are not automatically removed and continue to consume disk space.

## Interrupted or failed publication

Failures before the tenant pointer is replaced preserve the previous snapshot. Temporary staging is removed after ordinary errors, but process termination can leave orphan generations, temporary pointers and locks.

An error after pointer replacement can mean a complete new snapshot is visible while its durability after a crash is uncertain. Inspect the pointer and verify the referenced manifest and every artifact checksum before retrying or selecting a recovery generation. Cancellation racing with pointer replacement may still publish.

Reserved paths include `.isc-snapshots`, `.isc-pointer-*`, `.isc-lock-<tenant>`, tenant pointers and `manifest.json`. Before removing a stale lock, ensure no writer is active and verify the current generation. Never remove an active writer's lock.

Archive or remove an unreferenced generation only after confirming that no current pointer or reader uses it. For local rollback, stop writers, verify the desired generation and atomically replace the tenant symlink using operator tooling. This changes the local snapshot view; the exporter has no remote import or restore command.

## Mapping archives

`-retrieve-mappings` reads Configuration Hub mappings into a separate new directory. It does not run an export or backup evaluation, or modify an object snapshot. Verify the declared source and target, actual API origin, and mapping checksum in `provenance.json` before using an archive. See [mapping retrieval](MAPPINGS.md) for permissions, publication and recovery details.
