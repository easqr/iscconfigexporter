# ISC Configuration Exporter

A Go module and command-line tool for exporting SailPoint Identity Security Cloud (ISC) configuration to complete, deterministic filesystem snapshots. It uses the official SailPoint Go SDK and supports optional backup inventory and retention evaluation. **The supplied Configuration Hub adapter cannot delete backups**; see [API findings](docs/API.md) for the limitations.

The tool exports configuration; it does not import or restore it, compare environments, schedule exports, or publish them to Git or remote storage.

## Build and run

Requires Go 1.27.2 or newer and a local POSIX filesystem with symlink, hardlink, directory fsync and atomic rename support (tested on macOS). Network access is needed to download build dependencies. Run these commands from the repository root:

```sh
go build -o isc-exporter ./cmd/isc-exporter
cp exporter.example.json exporter.json
./isc-exporter -help
./isc-exporter -config exporter.json -validate
```

Edit the tenant, output directory and filters in `exporter.json`. Configure connection and authentication separately using the SDK's native `config.json` (for example, `sail sdk init config`) or its optional `SAIL_*` settings. See [native SDK setup](docs/CONFIGURATION.md#native-sdk-connection-and-authentication). Local `-validate` checks require no credentials or network access.

Run an export:

```sh
./isc-exporter -config exporter.json
```

Cleanup is disabled by default. Add `-cleanup-preview` to force non-destructive cleanup evaluation after export. Preview still performs a remote export and local publication. SIGINT/SIGTERM request cancellation; see the [configuration reference](docs/CONFIGURATION.md) for SDK authentication timeout limitations. Standard output contains a JSON result; standard error contains structured logs and failures. Cleanup failure returns a nonzero exit status after emitting the published result. Use `-help` for command-line options.

## Library usage

Module path: `gitlab.com/easqrtech/iam/iscconfigexporter`. This module is published on GitLab.

```go
c := exporter.DefaultConfig()
c.Tenant = "example"
c.Output = "exports"
api, err := sdk.New(c, nil)
if err != nil { return err }
service := exporter.Service{
    Engine: exporter.Engine{Config: c, API: api},
    Store: exporter.FileStore{Output: c.Output},
    Lifecycle: exporter.BackupManager{API: api.Backups(c)},
}
result, err := service.Export(ctx)
```

Import the root module as `exporter` and its `/sdk` package as `sdk`. The example assumes an existing `ctx` and returns errors to its caller. `sdk.New` loads the native SDK configuration; embedded applications may pass an existing `*sailpoint.Configuration` to `sdk.NewWithConfiguration(c, cfg, nil)` instead. `Engine.Fetch` and `Organize` can also be used independently. See [architecture](docs/ARCHITECTURE.md) for component interfaces and extension points.

## Output

```text
exports/
  example -> .isc-snapshots/example-<generation>
  .isc-snapshots/example-<generation>/
    ROLE/id-<hex-of-self.id>.json
    SOURCE/id-<hex-of-self.id>.json
    manifest.json
```

Object paths are derived from the export envelope's `self.type` and `self.id`:
`<self.type>/id-<lowercase hexadecimal encoding of the UTF-8 bytes of self.id>.json`.
The ID is read from `self.id`, rather than the nested `object.id` or `self.name`.
Each byte becomes two hexadecimal characters, so a 32-character ASCII ID produces
64 hexadecimal characters in the filename. This is a deterministic, reversible
encoding, not a generated GUID, a hash, or encryption.

For example, an envelope containing:

```json
{
  "version": 1,
  "self": { "type": "ROLE", "id": "abc123", "name": "Example role" },
  "object": {}
}
```

is written to `ROLE/id-616263313233.json`. Hex encoding preserves the exact ID
while avoiding special characters, substitution collisions, and case collisions
on filesystems that treat uppercase and lowercase filenames as equivalent.
The original `self.id` and `self.name` remain readable inside the JSON file.

Each object retains its complete exported envelope and unknown fields. Only map keys are ordered; array order and numeric precision are retained. Manifest records tenant, operation, completion time, selected types, counts, relative artifact paths, SHA-256 checksums and created/modified/unchanged/deleted paths. Filtered exports replace the current managed snapshot with the selected types. Unmanaged regular files are carried forward; collisions and nested symlinks fail closed.

Snapshots publish atomically through the tenant symlink. Consumers needing multi-file consistency must call CurrentPath once and use that resolved generation. Unchanged objects retain inode/mtime. Old generations remain for recovery and consume disk; treat all generations as read-only. See [architecture](docs/ARCHITECTURE.md) for durability, locking and recovery limits.

## Operations and security


- [Configuration reference](docs/CONFIGURATION.md)
- [Operational behavior and recovery](docs/OPERATIONS.md)
- [Live-tenant validation checklist](docs/LIVE_VALIDATION.md)
- [Dependency licenses](docs/DEPENDENCIES.md)

Configuration exports can contain secrets. Protect output storage and access; files use permissions 0600 and directories 0700. Exported payloads are preserved without redaction. No encryption or signature verification is provided. Exporter logs omit raw API error bodies and object payloads, and credentials are not written to exporter policy or manifests.

Use private, trusted storage for the output root and ownership registry, and a trusted HTTPS origin for the SDK connection. See the [configuration reference](docs/CONFIGURATION.md) for SDK authentication, logging, timeout, redirect and token-cache limitations. The source code has no assigned open-source license; dependency notices are linked above.




## SDK-free snapshot contract

`gitlab.com/easqrtech/iam/iscconfigexporter/snapshot` lets Go applications read and validate exported snapshots without depending on the SailPoint SDK. It provides manifest/artifact models, strict JSON decoding, canonical identity paths and pinned directory reading.

Readers choose `snapshot.SnapshotMode` (manifest-listed integrity-checked artifacts) or `snapshot.TreeMode` (current edited configuration inventory; manifest scope and historical provenance). A manifest-free tree must declare `Options.Types`. Root symlinks resolve once; nested inventory links and special files are rejected. See the sibling [export input contract](../configdiffer/docs/contracts.md) and [comparison policy/report contract](../configdiffer/docs/policy.md). Ordinary exports and the exporter CLI do not require the differ or Configuration Hub permissions.

## Optional read-only mapping retrieval

Configuration Hub Object Mappings can be retrieved separately into an archive containing `mappings.json` and `provenance.json`. Ordinary exports never request mappings or require their permissions. See [mapping retrieval](docs/MAPPINGS.md) for CLI examples, permissions, the library API and offline differ handoff.
