# ISC Configuration Exporter

A reusable Go module and thin CLI that retrieve SailPoint ISC configuration through the official Go SDK and publish complete, deterministic filesystem snapshots. Includes optional backup inventory, retention evaluation and a trusted ownership registry. **The supplied Configuration Hub adapter cannot delete backups**: verified ownership and conditional deletion guarantees are unavailable. See [API findings](docs/API.md).

No Git integration, import/restore, drift analysis, cross-tenant deployment, tokenization, scheduling, database, web service, daemon or remote publication is implemented.

## Build and run

Requires Go 1.27.2 or newer and a local POSIX filesystem with symlink, hardlink, directory fsync and atomic rename support (tested on macOS). Network access is needed to download pinned build dependencies. Tests require no tenant credentials or external services.

```sh
go build -o isc-exporter ./cmd/isc-exporter
cp config.example.json config.json
./isc-exporter -help
./isc-exporter -config config.json -validate
```

Edit tenant/base_url/output and filters in the configuration before live use. Credentials come exclusively from `ISC_CLIENT_ID` and `ISC_CLIENT_SECRET`, populated through your secret-management environment. Cleanup is disabled by default. Execute `./isc-exporter -config config.json` to export, or add `-cleanup-preview` to force non-destructive cleanup evaluation after export. Preview still performs a remote export and local publication. SIGINT/SIGTERM cancel work. Standard output is a JSON Result; standard error contains structured logs and failures. Cleanup failure returns nonzero after emitting the published result.

## Library usage

Module path: `github.com/elianderson/iscutilities/configexporter`. This repository has no remote and has not been published; use a local `replace` directive from another application until the owner assigns a canonical module location.

```go
c := exporter.DefaultConfig()
c.Tenant = "example"
c.BaseURL = "https://example.api.identitynow.com"
c.Output = "exports"
api, err := sdk.New(c, exporter.CredentialsFromEnv(), nil)
if err != nil { return err }
service := exporter.Service{
    Engine: exporter.Engine{Config: c, API: api},
    Store: exporter.FileStore{Output: c.Output},
    Lifecycle: exporter.BackupManager{API: api.Backups(c)},
}
result, err := service.Export(ctx)
```

Import the root module as `exporter` and its `/sdk` package as `sdk`. API, Store, Lifecycle, Registry and OwnershipVerifier are exporter-owned contracts; public signatures do not expose generated SDK models. Engine.Fetch and Organize can also be used independently.

## Output

```text
exports/
  example -> .isc-snapshots/example-<generation>
  .isc-snapshots/example-<generation>/
    ROLE/id-<hex-of-stable-id>.json
    SOURCE/id-<hex-of-stable-id>.json
    manifest.json
```

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

Configuration exports can contain sensitive tenant configuration. Protect output storage and access; artifacts use 0600 and directories 0700. SDK debug logging/retries are disabled. Credentials are never serialized into manifests or configuration; raw API error bodies and object payloads are never logged. Original exported payload fields are preserved, so tenant configuration itself may contain secrets. No encryption or signature verification is provided. Base URL must be a trusted HTTPS origin and redirects are disabled. Output root and registry are trusted private storage, not hostile shared directories. The source code is not assigned an open-source license by this implementation.
