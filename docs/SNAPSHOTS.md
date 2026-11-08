# Snapshot files and readers

A snapshot is a saved set of exported configuration records plus `manifest.json`. This reference describes the exporter file format and Go reader API. For running an export, start with the [README](../README.md); for storage failures and recovery, see [operations](OPERATIONS.md).

## Saved versions and the current snapshot

Each successful export creates a new directory under `<output>/.isc-snapshots/`. This saved version is called a **generation**. The symbolic link `<output>/<tenant>` points to the current generation. Publication switches that link only after the new snapshot has been written, so readers see a complete old or new snapshot.

Keep all generations read-only. Unchanged files can share storage through hardlinks, so editing a file can also change older generations. Older generations remain on disk until an operator archives or removes them. A filtered export replaces the current set of exporter-written files with exactly the selected types; it does not remove old generations.

Files listed in the previous manifest are **managed files**: the exporter tracks and replaces or removes them during the next export. Other regular files in the current generation are unmanaged and are copied forward. If a new export would overwrite one of those files, publication fails. Nested symbolic links and special files are also rejected.

## Object files and filenames

Each object file contains its complete exported **envelope**: a JSON wrapper with `version`, `self` metadata and an `object` payload. Unknown fields and signatures are preserved. JSON map keys are ordered for consistent output; array order and numeric precision are retained. Signatures are not verified or regenerated.

The record's `self.type` selects its directory. Its `self.id` selects the filename:

```text
<self.type>/id-<lowercase hexadecimal encoding of the UTF-8 bytes of self.id>.json
```

The ID comes from `self.id`, rather than `object.id` or `self.name`. Each byte becomes two hexadecimal characters, so a 32-character ASCII ID produces 64 hexadecimal characters in the filename. This encoding is reversible; it is not a generated GUID, hash or encryption.

For example:

```json
{
  "version": 1,
  "self": { "type": "ROLE", "id": "abc123", "name": "Example role" },
  "object": {}
}
```

This record is written to `ROLE/id-616263313233.json`. Encoding preserves the exact ID while avoiding filename substitutions and collisions on filesystems that treat uppercase and lowercase names as equivalent. The original ID and name remain readable in the JSON file.

Tenant and type names must follow the [identifier rules](CONFIGURATION.md#identifier-rules). Object IDs must contain 1–100 bytes and cannot contain `/`, `\`, control characters, or be exactly `.` or `..`. Duplicate object paths and paths that differ only by letter case are rejected.

`exporter.BuildSnapshot` accepts positive integer envelope versions; the Go snapshot reader supports version `1` only. This difference is tracked in [envelope-version issue #4](https://gitlab.com/easqrtech/iam/iscconfigexporter/-/work_items/4).

## Manifest

Manifest schema `1` describes the published snapshot:

| JSON field | Meaning |
| --- | --- |
| `schema` | Exporter manifest format version, currently `1` |
| `tenant` | Tenant identity declared by the export |
| `job_id` | Remote export job ID |
| `completed` | Exporter-generated completion timestamp in UTC |
| `types` | Selected object types, including types with zero objects |
| `counts` | Object count for each selected type |
| `artifacts` | Object-file paths relative to the generation, with lowercase SHA-256 checksums |
| `changes` | Paths created, modified, unchanged or deleted relative to the previous snapshot |

A checksum lets a reader check whether file bytes match the manifest; it does not prove who created them. The manifest contains no signature verification or separate success-status field. See [operations](OPERATIONS.md#results-and-errors) for an illustrative CLI result containing a manifest.

## Go readers

The `gitlab.com/easqrtech/iam/iscconfigexporter/snapshot` package depends only on the Go standard library. It owns `Artifact`, `Changes`, `Manifest` and `Snapshot`, and provides decoding, checksum, path and reader functions.

Use `snapshot.Read(path, options)` to read from disk, or `snapshot.FromFiles(files, options)` to validate a map of relative filenames to bytes. Choose an explicit mode:

| Mode | Use it for | Manifest behavior |
| --- | --- | --- |
| `snapshot.SnapshotMode` | An untouched exported snapshot | Manifest required; only listed files are read, and checksums, counts, identities and types must match |
| `snapshot.TreeMode` | A separate working copy with intentional edits, additions or deletions | A supplied manifest sets the selected types; old counts and checksums describe the original export, not current files |

For a tree without a manifest, explicitly set `Options.Types`, even when it contains no objects. A type override alongside a manifest is rejected. A malformed manifest is an error in either mode; a failed snapshot read never silently switches to tree mode.

Readers validate canonical paths, exact manifest field spellings, envelope version `1`, object identities and selected types. A payload `object.id`, if present, must match `self.id`. Duplicate JSON keys, invalid UTF-8, conflicting identities and unsupported versions are errors. Unknown object fields are retained.

On disk, the root symbolic link is resolved once and the directory is held open for subsequent reads. Nested symbolic links and special files within the inventory are rejected. For a manual multi-file reader, Go applications can call the root package's `CurrentPath(output, tenant)` once, then use the returned path for every read. Resolving the tenant link anew for each file can mix generations if an export finishes between reads.

Tree mode ignores ordinary non-JSON files and root `.git`, `.hg` and `.svn` administration entries, but rejects linked or special administration entries. Other JSON files must be valid object files; keep policies and mapping archives outside the working tree. Avoid concurrent edits while reading a working tree: change checks can detect interference but cannot make an actively edited directory a consistent snapshot.

## Use with configdiffer

The separate configdiffer project consumes these snapshots for offline comparison. Its *Export input contract* and *Comparison policy v2* documents describe comparison-specific scope, identity and policy rules. Configdiffer is not published yet; those documents are available in its source checkout. Exporting and reading snapshots do not require configdiffer or Configuration Hub mapping permissions.
