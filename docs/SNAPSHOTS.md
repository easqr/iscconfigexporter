# Snapshot files and readers

A snapshot is a saved set of exported configuration records plus `manifest.json`. This reference describes the exporter file format and Go reader API. For running an export, start with the [README](../README.md); for storage failures and recovery, see [operations](OPERATIONS.md).

## Saved versions and the current snapshot

Each successful export creates a new directory under `<output>/.isc-snapshots/`. This saved version is called a **generation**. The symbolic link `<output>/<tenant>` points to the current generation. Publication switches that link only after the new snapshot has been written, so readers see a complete old or new snapshot.

Keep all generations read-only. Unchanged files can share storage through hardlinks, so editing a file can also change older generations. Older generations remain on disk until an operator archives or removes them. A filtered export replaces the current set of exporter-written files with exactly the selected types; it does not remove old generations.

Files listed in the previous manifest are **managed files**: the exporter tracks and replaces or removes them during the next export. Other regular files in the current generation are unmanaged and are copied forward. If a new export would overwrite one of those files, publication fails. Nested symbolic links and special files are also rejected.

## Object files and filenames

Each object file contains its complete exported **envelope**: a JSON wrapper with `version`, `self` metadata and an `object` payload. Unknown fields and signatures are preserved. JSON map keys are ordered for consistent output; array order and numeric precision are retained. Signatures are not verified or regenerated.

The record's `self.type` selects its directory. By default, its `self.id` selects the filename:

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

Building, publishing and reading snapshots require envelope version `1`. A payload `object.id`, when present, must match `self.id`; payloads without `object.id` are supported. Unsupported versions and conflicting identities are rejected before publication.

## Filename compatibility

The policy's `filename_mode` chooses the filename within each type directory:

| Mode | Path for ID `abc123`, name `Example role` | Manifest |
| --- | --- | --- |
| `hex` (default) | `ROLE/id-616263313233.json` | Schema 1, unchanged |
| `id` | `ROLE/abc123.json` | Schema 2, `filename_mode: "id"` |
| `name` | `ROLE/Example role.json` | Schema 5, `filename_mode: "name"` |

Name mode defaults to no ID suffix. If filenames collide after escaping and
profile prefixes, all members of that collision group get `--<self.id>` before
`.json`. Comparison includes case and Unicode equivalence. The planner repeats
collision detection so a generated suffix cannot overwrite another object's
original name. Set `"filename_append_id": true` in `exporter.json` to force IDs on
every name-mode file; the default is false. If IDs still do not distinguish the
filenames, publication fails with a collision report. Changing inventory can add
or remove suffixes on subsequent exports; force IDs for stable suffix behavior.

For `LIFECYCLE_STATE`, name mode also prepends the identity profile name:

```text
LIFECYCLE_STATE/Employees--Active.json
```

The prefix comes from `object.identityProfileRef.name`, requiring an explicit
reference with type `IDENTITY_PROFILE` and a nonempty ID and name. The parent
profile object need not be included in the export. A missing or malformed reference
produces a `missing_identity_profile` compatibility issue in name mode; there is no
inferred prefix or extra API lookup. Escaping and filename-length checks apply to
the complete profile--state stem, including any ID suffix. Renaming the profile changes its lifecycle-state
paths in the next name-mode export.

The words Active, Inactive and Terminated are valid filename content; the profile
prefix supplies context, and IDs distinguish any remaining filename collisions.

Without escaping, values are used exactly. Name mode reads
`self.name`; a missing, non-string or empty name is an issue, with no fallback to
`object.name` or the ID. All modes retain the existing object-ID validation rules.
Ordinary spaces, parentheses, ampersands and similar valid punctuation are preserved.
A trailing dot or space in a value is internal to the completed filename.

Set `"filename_escape": true` in `exporter.json` to percent-encode unsafe characters
as uppercase UTF-8 byte escapes: colon becomes `%3A`, slash `%2F`, and backslash
`%5C`. Literal percent signs always become `%25`, so a literal `%3A` cannot collide
with an encoded colon. Leading ASCII spaces and control characters are escaped;
for reserved Windows device names such as `NUL.txt`, the first byte is escaped
(`%4EUL.txt`). Other valid punctuation and Unicode stay readable. Escaping operates
on the completed stem, including the ID suffix in name mode. It never modifies the
object envelope. It defaults to false and has no effect in hex mode.

Escaped ID files use manifest schema 3; all new name-mode files use schema 5,
recording `filename_mode` and `filename_escape` (omitted when false). Strict ID files
retain schema 2; hex files retain schema 1. Schema 2's older name-only convention and
schema 3's name--ID convention without profile prefixes, and schema 4's
always-suffixed profile-aware convention remain readable. The next
name-mode export reconciles those managed paths to the new convention.

Before publication, ID/name modes reject any remaining separators, Windows-reserved punctuation,
NUL and other control characters, Windows device names (including names with
extensions and superscript device digits), leading ASCII spaces, and filenames
longer than 255 UTF-8 bytes or 255 UTF-16 code units including `.json`. UTF-8 length
is a conservative component limit. Collision checks group files within each type
using canonical Unicode normalization and full Unicode case folding. This is
intentionally conservative: it can reject pairs that a particular filesystem would
allow. Names shared by different types do not collide. Duplicate object identities
are rejected independently of filenames.

`-check-filenames` prints `tenant`, `job_id`, `objects` and a `modes` array for `id`
and `name`. Each mode has `escape`, `append_id`, `safe`, `files` and `issues`. The check applies the policy's
`filename_escape` setting. `files` lists every proposed path, including an `escaped`
boolean indicating whether encoding changed its stem, plus `id_appended` to
show whether the file received an ID suffix. Each issue lists `code`, `type`, `id`,
`name`, proposed `path`, `platforms` and an explanation. Issues are sorted by type,
ID and code. Exit status is 1 if either mode has issues. This action creates and
downloads a remote ISC export, respecting type filters, but never publishes files
or evaluates backups. Report values describe the inventory, not object payloads.

Checks assess filename components and inventory collisions regardless of the host.
They do not certify arbitrary destination full-path lengths, mounted filesystem
behavior, Finder display behavior or storage support. Windows publication remains
unsupported by `FileStore` independently of filename compatibility.

Changing the naming mode uses the normal atomic publication process; previously
managed paths are reconciled, unmanaged files are preserved, and collisions with
unmanaged files fail publication. In name mode, renaming an object creates a new
path and deletes its old managed path, while keeping the object's ID unchanged.

Updated readers accept schemas 1 through 5 and validate each filename against its envelope
and declared mode. Older readers may reject newer schemas; consumers such as configdiffer
must use an exporter dependency containing this reader update. For manifest-free
working trees, set `Options.FilenameMode` explicitly for ID/name files and
`Options.FilenameEscape` for escaped files and `Options.FilenameAppendID` to
force suffixes; omission means strict hex. Manifest-free
name trees use the new profile-aware name--ID convention; retain the original
schema 2, 3 or 4 manifest to read a legacy tree. With a manifest, supplied options must agree. Snapshot mode verifies the suffix
plan against the entire inventory. Tree mode permits a valid historical suffix
when a peer has been deleted from a working copy. After changing `self.name`
in a name-mode working copy, rename its file to match.

Go callers use `BuildSnapshotWithFilenameOptions`, `CheckFilenamesWithOptions`,
or `Service.CheckFilenames`. The strict wrappers `BuildSnapshotWithNaming` and
`CheckFilenames` leave escaping disabled. `BuildSnapshot` retains its existing hex behavior.
Compatibility failures during export are available as `*snapshot.FilenameError`
through `errors.As`, including through an `*exporter.ExportError` wrapper.

## Manifest

Manifest schemas `1` through `5` describe the published snapshot:

| JSON field | Meaning |
| --- | --- |
| `schema` | Exporter manifest format version: `1` for hex, `2` for literal ID/legacy name, `3` for escaped ID/legacy name--ID, `4` for always-suffixed profile-aware names, `5` for conditional suffixes |
| `filename_mode` | Required `id` or `name` in schemas 2-5; omitted in schema 1 |
| `filename_escape` | Schema 3-5 percent escaping; omitted when false; forbidden as true in older schemas |
| `filename_append_id` | Schema 5: force IDs on all name-mode files; omitted/false means suffix collision groups only |
| `tenant` | Tenant identity declared by the export |
| `job_id` | Remote export job ID |
| `completed` | Exporter-generated completion timestamp in UTC |
| `types` | Selected object types, including types with zero objects |
| `counts` | Object count for each selected type |
| `artifacts` | Object-file paths relative to the generation, with lowercase SHA-256 checksums |
| `changes` | Paths created, modified, unchanged or deleted relative to the previous snapshot |

A checksum lets a reader check whether file bytes match the manifest; it does not prove who created them. The manifest contains no signature verification or separate success-status field. See [operations](OPERATIONS.md#results-and-errors) for an illustrative CLI result containing a manifest.

## Go readers

For a complete read example, see the [Go library guide](LIBRARY.md#read-a-saved-snapshot).

The `github.com/easqr/iscconfigexporter/snapshot` package uses the Go standard library and `golang.org/x/text` for portable Unicode collision checks, without the SailPoint SDK. It owns `Artifact`, `Changes`, `Manifest` and `Snapshot`, and provides decoding, checksum, path and reader functions.

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

The separate configdiffer project consumes these snapshots for offline comparison. Its *Export input contract* and *Comparison policy v2* documents describe comparison-specific scope, identity and policy rules. Those documents are available in its source checkout. Exporting and reading snapshots do not require configdiffer or Configuration Hub mapping permissions.
