# ISC Configuration Exporter

A Go module and command-line tool for exporting SailPoint Identity Security Cloud (ISC) configuration to local snapshots. Each snapshot contains the exported configuration files and a manifest describing them. It uses the official SailPoint Go SDK and supports optional read-only backup evaluation. The supplied integration cannot delete backups; see [API capabilities and limits](docs/API.md).

The tool exports configuration; it does not import or restore it, compare environments, schedule exports, or publish them to Git or remote storage.

## Build and run

Requires Go 1.27.3 or newer, network access to download build dependencies, and supported local storage. The exporter is tested on macOS; Windows and network filesystems are unsupported. See [storage requirements](docs/OPERATIONS.md#snapshot-storage).

Run these steps from the repository root:

1. Build the command-line tool:

   ```sh
   go build -o isc-exporter ./cmd/isc-exporter
   ```

2. Create the exporter policy:

   ```sh
   cp exporter.example.json exporter.json
   ```

   Edit `exporter.json` to set your tenant, output directory and type filters. See the [configuration reference](docs/CONFIGURATION.md) for policy fields.

3. Configure the SDK connection and credentials in a separate `config.json`, or use `SAIL_*` environment variables. Follow [SDK connection setup](docs/CONFIGURATION.md#native-sdk-connection-and-authentication). `exporter.json` does not contain credentials.

4. Check the local policy and options:

   ```sh
   ./isc-exporter -config exporter.json -validate
   ```

   Expect `configuration valid`. This check does not load credentials, contact ISC, or verify API access.

5. Run an export:

   ```sh
   ./isc-exporter -config exporter.json
   ```

   A successful export writes a JSON result to standard output and saves a snapshot under the configured output directory. With tenant `example` and the default output directory, look for `exports/example/manifest.json`. See [results and errors](docs/OPERATIONS.md#results-and-errors) for a sample result and exit-status behavior.

Use `./isc-exporter -help` for command-line options. Backup evaluation is disabled by default. `-cleanup-preview` evaluates an enabled backup policy after exporting; it still creates a remote export job and saves a local snapshot. See [backup evaluation](docs/CONFIGURATION.md#backup-evaluation).

Run `./isc-exporter -version` to see the exporter version, full commit ID, checkout modification state, Go version and platform. This command needs no configuration or credentials. Unavailable commit or modification information is shown as `unknown`.

Ctrl+C or SIGTERM requests cancellation. See [authentication limits](docs/CONFIGURATION.md#authentication-limits) for token requests that cannot be interrupted.

## Output

```text
exports/
  example -> .isc-snapshots/example-<generation>
  .isc-snapshots/example-<generation>/
    ROLE/id-<hex-of-self.id>.json
    SOURCE/id-<hex-of-self.id>.json
    manifest.json
```

`exports/example` is a symbolic link to the latest saved snapshot. Each object file contains its exported configuration record, including the original metadata and any unknown fields. Filenames encode the record's ID; the original ID and name remain readable inside the JSON.

The manifest lists exported types, object counts, file checksums and changed paths. Type filters determine which types appear in the current snapshot. Unchanged files retain their modification times, and older snapshot directories remain on disk for recovery.

Keep snapshot files read-only; use a separate copy if you need to edit configuration. For filename rules, manifest fields and Go reader modes, see [snapshot files and readers](docs/SNAPSHOTS.md). For disk management and recovery, see [operations](docs/OPERATIONS.md).

## Library usage

Module path: `github.com/easqr/iscconfigexporter`. The repository moved from GitLab; update existing imports to this path. Tags `v0.6.0` and `v0.7.0` retain their original GitLab module path. Until a new release is tagged, use `go get github.com/easqr/iscconfigexporter@main` for the migrated module.

The following function uses SDK connection settings configured as above and returns the export result or an error:

```go
package example

import (
    "context"

    exporter "github.com/easqr/iscconfigexporter"
    "github.com/easqr/iscconfigexporter/sdk"
)

func exportConfiguration(ctx context.Context) (exporter.Result, error) {
    c := exporter.DefaultConfig()
    c.Tenant = "example"
    c.Output = "exports"
    api, err := sdk.New(c, nil)
    if err != nil {
        return exporter.Result{}, err
    }
    service := exporter.Service{
        Engine: exporter.Engine{Config: c, API: api},
        Store: exporter.FileStore{Output: c.Output},
    }
    return service.Export(ctx)
}
```

Go applications can also supply SDK configuration with `sdk.NewWithConfiguration`, or use `Engine.ExportAndDownload` and `BuildSnapshot` separately. See [architecture](docs/ARCHITECTURE.md) for component interfaces and extension points, and [snapshot files and readers](docs/SNAPSHOTS.md#go-readers) for reading snapshots without the SailPoint SDK.

## Optional mapping retrieval

Configuration Hub Object Mappings can be retrieved separately into an archive containing `mappings.json` and `provenance.json`. Ordinary exports never request mappings or require their permissions. See [mapping retrieval](docs/MAPPINGS.md) for CLI examples, permissions, the library API and offline differ handoff.

## Operations and security

- [Documentation overview](docs/README.md)
- [Configuration reference](docs/CONFIGURATION.md)
- [Operations and troubleshooting](docs/OPERATIONS.md)
- [Live-tenant validation checklist](docs/LIVE_VALIDATION.md)
- [Dependency licenses](docs/DEPENDENCIES.md)

Configuration exports can contain secrets. Protect output storage and access; files use permissions 0600 and directories 0700, allowing access only to the owning user. Exported payloads are preserved without redaction. No encryption or signature verification is provided. Exporter logs omit raw API error bodies and object payloads, and SDK credentials are not written to exporter policy or manifests.

Use private storage under your control and an HTTPS SDK connection URL you trust. See [configuration](docs/CONFIGURATION.md) for authentication and logging limitations.

## License

The original configexporter source code and documentation are licensed under the [Apache License, Version 2.0](LICENSE). Third-party dependencies retain their own licenses; see [dependency licenses and notices](docs/DEPENDENCIES.md).
