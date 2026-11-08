# Execution and recovery

## Export lifecycle

An export validates the policy, discovers and selects supported types, starts one job, polls for completion, downloads and validates the objects, then publishes a complete snapshot. Optional backup evaluation runs after the new snapshot is saved.

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
| `cleanup_error` | Backup evaluation failed after the export was saved; present only on failure |

The CLI writes the saved export result before returning a nonzero exit status for a backup evaluation failure. An ownership limitation in `cleanup.limitation` is not itself an export failure. The supplied SDK adapter cannot delete backups, regardless of the retention policy. See [API capabilities and limits](API.md).

### Example result

This illustrative result represents a first export that selected `ROLE` but returned no objects, with backup evaluation disabled. The job ID and timestamp are fictional; this is not a record of live validation.

```json
{
  "manifest": {
    "schema": 1,
    "tenant": "example",
    "job_id": "example-job-id",
    "completed": "2026-11-08T18:00:00Z",
    "types": ["ROLE"],
    "counts": {"ROLE": 0},
    "artifacts": [],
    "changes": {
      "created": null,
      "modified": null,
      "unchanged": null,
      "deleted": null
    }
  },
  "cleanup": {"decisions": null}
}
```

For nonempty exports, `artifacts` lists file paths and checksums, and each populated change category contains paths. Empty change categories and backup decisions can be `null`. See [manifest fields](SNAPSHOTS.md#manifest) for the file-format reference.

## Troubleshooting

The table uses message fragments; an error can also include the failed operation or a local file path. Correct the setting or storage issue before running another export.

| Message or symptom | Meaning | Next action |
| --- | --- | --- |
| `cannot open exporter policy` | The file selected by `-config` is missing or unreadable | Copy `exporter.example.json`, edit it and select the file with `-config`; check file permissions |
| `policy field ... has the wrong JSON type`, or a named policy-field error | The policy contains an unsupported value or type | Check the named field in [configuration](CONFIGURATION.md), then run `-validate` again |
| `cannot load SDK connection settings` | No usable SDK `config.json` was selected, or the selected file could not be read or decoded | Check the exact filename, location and JSON format; if `SAIL_BASE_URL` is set, supply the complete environment configuration |
| `SDK ClientId and ClientSecret are required` | The selected connection settings lack credentials and no token was supplied | Configure both credentials in the selected SDK file or environment source; do not put them in exporter policy |
| `HTTP 401` / `HTTP 403` | Authentication failed / the operation is not authorized | Check credentials for 401; check the tenant role and required API permissions for 403 |
| `requires -mapping-source-org` / `requires -mapping-output` | Mapping retrieval is missing a required option | Supply a source organization (or `default`) and a new archive directory outside the export directory |
| `cannot lock the tenant output` | Another export may be running, a stale lock may remain, or the output directory is not writable | Check for an active writer and directory permissions; follow [interruption recovery](#interrupted-or-failed-publication) before removing any lock |
| `a file in the previous snapshot was changed or removed` | An exporter-written file no longer matches the previous manifest | Preserve local edits; restore a verified snapshot or choose a fresh output directory. Keep editable copies separate |
| `an export file would overwrite an unmanaged local file` | A file not tracked by the exporter occupies a required path | Preserve and move the conflicting file, or choose a fresh output directory |
| A nonzero exit status with a JSON result containing `cleanup_error` | The export was saved, then backup evaluation failed | Keep the saved result; check Configuration Hub read access before repeating; custom Go integrations should also check their ownership registry |
| `the new snapshot is visible, but syncing its directory failed` | The completed snapshot is visible, but its survival after a crash is uncertain | Verify the current generation and checksums before retrying; see recovery below |
| `operation timed out` / `operation canceled` | The local workflow reached its time limit or received a cancellation request | Inspect the current snapshot and any started ISC job before retrying; review [authentication limits](CONFIGURATION.md#authentication-limits) |

## Snapshot storage

Each export saves a version in its own directory, called a generation. A tenant symbolic link points to the current generation. Resolve that link once when reading several files; Go applications can use `CurrentPath`. Looking up the link for each file can mix versions during an export. See [snapshot files and readers](SNAPSHOTS.md) for the format and reader modes.

Treat generation files as read-only. Unchanged artifacts can be hardlinked across generations, so editing one can alter older snapshots too. Changed or missing managed files prevent the next publication.

Only files listed in the previous manifest are tracked for replacement or removal. Other regular files in the current generation are copied forward; files elsewhere in the output directory are untouched. Name collisions, nested symlinks, special files, invalid manifests, and an ordinary directory at the tenant pointer path block publication. Preserve any local edits before resolving a conflict or choosing a fresh output root.

Use private local storage under your control. It must support POSIX symbolic links, hardlinks, atomic rename and directory sync; Windows and network filesystems are unsupported. Capacity, quota and permission failures can prevent publication. Old and orphan generations remain on disk and require manual management. Retention follow-up is tracked in [snapshot retention issue #3](https://gitlab.com/easqrtech/iam/iscconfigexporter/-/work_items/3).

## Interrupted or failed publication

Failures before the tenant symbolic link is replaced preserve the previous snapshot. Temporary staging is removed after ordinary errors, but process termination can leave orphan generations, temporary pointers and locks.

An error after the tenant link is replaced can mean a complete new snapshot is visible while its durability after a crash is uncertain. Inspect the pointer and verify the referenced manifest and every artifact checksum before retrying or selecting a recovery generation. Cancellation arriving during that replacement may still publish.

Reserved paths include `.isc-snapshots`, `.isc-pointer-*`, `.isc-lock-<tenant>`, tenant pointers and `manifest.json`. Before removing a stale lock, ensure no writer is active and verify the current generation. Never remove an active writer's lock.

Archive or remove an unreferenced generation only after confirming that no current pointer or reader uses it. For local rollback, stop writers, verify the desired generation and atomically replace the tenant symlink using operator tooling. This changes the local snapshot view; the exporter has no remote import or restore command.

## Mapping archives

`-retrieve-mappings` reads Configuration Hub mappings into a separate new directory. It does not run an export or backup evaluation, or modify an object snapshot. Verify the declared source and target, actual API origin, and mapping checksum in `provenance.json` before using an archive. See [mapping retrieval](MAPPINGS.md) for permissions, publication and recovery details.
