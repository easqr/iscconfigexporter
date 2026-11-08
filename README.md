# ISC Configuration Exporter

A reusable Go module and thin CLI that retrieve SailPoint ISC configuration through the official Go SDK and publish complete, deterministic filesystem snapshots. Includes optional backup inventory, retention evaluation and a trusted ownership registry. **The supplied Configuration Hub adapter cannot delete backups**: verified ownership and conditional deletion guarantees are unavailable. See [API findings](docs/API.md).

No Git integration, import/restore, drift analysis, cross-tenant deployment, tokenization, scheduling, database, web service, daemon or remote publication is implemented.

## Build and run

Requires Go 1.27.2 or newer and a local POSIX filesystem with symlink, hardlink, directory fsync and atomic rename support (tested on macOS). Network access is needed to download pinned build dependencies. Tests require no tenant credentials or external services.

```sh
go build -o isc-exporter ./cmd/isc-exporter
cp exporter.example.json exporter.json
./isc-exporter -help
./isc-exporter -config exporter.json -validate
```

Edit tenant/output and filters in `exporter.json`. Configure connection and authentication using the SDK's native `config.json` (for example, `sail sdk init config`) or its optional `SAIL_*` settings. A secret manager and exporter-specific credential variables are not required. See [native SDK setup](docs/CONFIGURATION.md#native-sdk-connection-and-authentication). Cleanup is disabled by default. Execute `./isc-exporter -config exporter.json` to export, or add `-cleanup-preview` to force non-destructive cleanup evaluation after export. Preview still performs a remote export and local publication. SIGINT/SIGTERM cancel work. Standard output is a JSON Result; standard error contains structured logs and failures. Cleanup failure returns nonzero after emitting the published result.

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

Import the root module as `exporter` and its `/sdk` package as `sdk`. `sdk.New` calls `sailpoint.NewDefaultConfiguration`; embedded applications may pass an existing native `*sailpoint.Configuration` to `sdk.NewWithConfiguration(c, cfg, nil)` instead. SDK types are confined to that integration boundary. API, Store, Lifecycle, Registry and OwnershipVerifier are exporter-owned contracts; public signatures do not expose generated SDK models. Engine.Fetch and Organize can also be used independently.

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

## Validation and operations

```sh
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
```

Tests cover real SDK requests using fake HTTP transports, external-package library use, multiple types, 3,000 synthetic objects, duplicate/unsafe inputs, polling/cancellation/deadlines, publication failures, repeated export/reconciliation, concurrent writers, backup policies and CLI validation. No live-tenant validation has been performed.

- [Configuration reference](docs/CONFIGURATION.md)
- [Operational behavior and recovery](docs/OPERATIONS.md)
- [Owner-run live validation plan](docs/LIVE_VALIDATION.md)
- [Dependency licenses](docs/DEPENDENCIES.md)
- [Implementation progress](PROGRESS.md)

Configuration exports can contain sensitive tenant configuration. Protect output storage and access; artifacts use 0600 and directories 0700. SDK debug/HTTP logging is disabled, and SDK retries are restricted to transient GET responses. Credentials are never serialized into exporter policy or manifests; raw API error bodies and object payloads are never logged. Original exported payload fields are preserved, so tenant configuration itself may contain secrets. No encryption or signature verification is provided. Native SDK BaseURL must be a trusted HTTPS origin; API redirects are disabled. The pinned SDK's token acquisition has separate timeout, redirect and token-cache limitations documented in the configuration reference. Output root and registry are trusted private storage, not hostile shared directories. The source code is not assigned an open-source license by this implementation.


## Migration from the initial implementation

Rename your exporter policy to `exporter.json` and remove `base_url`; the SDK now supplies the connection URL. Reserve `config.json` for SDK configuration, rather than copying the old exporter policy there. Replace calls to `sdk.New(c, exporter.CredentialsFromEnv(), transport)` with `sdk.New(c, transport)` or `sdk.NewWithConfiguration(c, nativeConfig, transport)`. The exporter-owned Credentials type/loader and ISC_CLIENT_ID/ISC_CLIENT_SECRET settings have been removed. Native SDK defaults, precedence and limitations are described in the configuration reference. The [SDK reuse audit](docs/SDK_REUSE_AUDIT.md) records the whole-codebase review.

## SDK-free snapshot contract

`gitlab.com/easqrtech/iam/iscconfigexporter/snapshot` exposes manifest/artifact models, strict JSON decoding, canonical identity paths, pinned directory reading, and exporter-format in-memory validation. It imports only the Go standard library. Existing root `Artifact`, `Changes`, `Manifest`, and `Snapshot` remain compatible aliases; root organization and export APIs are unchanged.

Readers choose `snapshot.SnapshotMode` (manifest-listed integrity-checked artifacts) or `snapshot.TreeMode` (current edited configuration inventory; manifest scope and historical provenance). A manifest-free tree must declare `Options.Types`. Root symlinks resolve once; nested inventory links and special files are rejected. See the sibling [export input contract](../configdiffer/docs/contracts.md) and [comparison policy/report contract](../configdiffer/docs/policy.md). Ordinary exports and the exporter CLI do not require the differ or Configuration Hub permissions.

## Optional read-only mapping retrieval

Retrieve Configuration Hub Object Mappings separately from configuration exports:

```sh
isc-exporter -config exporter-sandbox.json -retrieve-mappings \
  -mapping-source-org invented-prod \
  -mapping-output ./mapping-archives/prod-to-sandbox-001
```

The SDK connection and policy tenant describe the target/current tenant. A new, separate directory receives original `mappings.json` bytes and checksum-bound `provenance.json`; stdout contains publication metadata only. Use `default` for an unassociated source group. Ordinary exports never request mappings or require their permissions. See [mapping retrieval](docs/MAPPINGS.md) for scope, native SDK reuse, local validation, the library API, safe publication, and offline differ handoff.
