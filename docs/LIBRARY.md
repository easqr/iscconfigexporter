# Use the Go library

Use this guide to export configuration from your Go application or read an
existing snapshot. For the command-line walkthrough, see the [README](../README.md).

## Install the module

Use Go 1.27.3 or newer. For a new application, create a directory and initialize
its Go module before adding the exporter:

```sh
mkdir isc-export-example
cd isc-export-example
go mod init example.com/isc-export-example
go get github.com/easqr/iscconfigexporter@v0.8.0
```

For an existing application, run just the `go get` command from its module directory.

Choose packages based on what your application needs:

| Package | Use it to |
| --- | --- |
| `github.com/easqr/iscconfigexporter` | Configure exports, build snapshots, and save them through library interfaces |
| `github.com/easqr/iscconfigexporter/sdk` | Connect the exporter to the official SailPoint SDK |
| `github.com/easqr/iscconfigexporter/snapshot` | Read and validate saved snapshots using only the Go standard library |

## Export configuration

First set up the SDK's `config.json` or `SAIL_*` variables using the
[connection guide](CONFIGURATION.md#native-sdk-connection-and-authentication).
For a Go application, file lookup starts beside the running executable, then
uses its working directory. You can instead pass an SDK configuration to
`sdk.NewWithConfiguration`.

Save this example as `main.go` in your application. Replace `example` with your
tenant's identity and choose the supported types you want to export:

```go
package main

import (
    "context"
    "fmt"
    "log"

    exporter "github.com/easqr/iscconfigexporter"
    "github.com/easqr/iscconfigexporter/sdk"
)

func main() {
    c := exporter.DefaultConfig()
    c.Tenant = "example"
    c.Output = "exports"
    c.Include = []string{"ROLE", "SOURCE"}

    api, err := sdk.New(c, nil)
    if err != nil {
        log.Fatal(err)
    }
    service := exporter.Service{
        Engine: exporter.Engine{Config: c, API: api},
        Store:  exporter.FileStore{Output: c.Output},
    }
    result, err := service.Export(context.Background())
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Saved %d files for %s (job %s)\n",
        len(result.Manifest.Artifacts), result.Manifest.Tenant, result.Manifest.JobID)
}
```

Run it with `go run .` after configuring credentials. `Service.Export` applies
`c.Timeout` to the workflow; an earlier caller-context deadline also applies.
The SDK's native token requests have separate
[timeout and refresh limitations](CONFIGURATION.md#authentication-limits).

This example uses default settings with backup evaluation disabled. Set
`c.Include` to `nil` to select all discovered types. The output directory has the
same storage requirements and read-only snapshot rules as the CLI.

### Handle failures in an application

The example stops on an export error. Applications that retry should inspect the
result and error first:

- `err != nil` means export or publication failed. `result.Manifest` can still
  describe a visible snapshot if a storage error occurred after publication.
  Inspect the stored snapshot before retrying.
- A wrapped `*exporter.ExportError` exposes a known remote `JobID`. Use
  `errors.As` to retrieve it and `errors.Is` to inspect the underlying cause.
  Cancellation does not cancel that remote job.
- When a custom backup lifecycle is configured, `result.CleanupError` reports a
  backup evaluation failure separately from `err`. A saved snapshot is retained.
  `result.Cleanup.Limitation` describes a capability limit and is not itself a
  failure.

See [results and recovery](OPERATIONS.md) for the operational consequences.

## Read a saved snapshot

Reading a snapshot is offline and needs no SDK connection or credentials. The
following example validates an untouched export and prints its tenant and object
count:

```go
package main

import (
    "fmt"
    "log"

    "github.com/easqr/iscconfigexporter/snapshot"
)

func main() {
    input, err := snapshot.Read("exports/example", snapshot.Options{
        Mode: snapshot.SnapshotMode,
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("%s: %d objects\n", input.Manifest.Tenant, len(input.Objects))
}
```

Use `snapshot.SnapshotMode` for an untouched export. Use `snapshot.TreeMode` for a
separate working copy that you intentionally edit. For a manifest-free tree,
also set `Options.Types`; do not switch modes just to bypass an integrity error.
`snapshot.FromFiles` applies validation to files already held in memory.

See [snapshot files and readers](SNAPSHOTS.md#go-readers) for mode rules, scope,
canonical filenames, and input validation.

## Other library workflows

- Use `Engine.ExportAndDownload` and `BuildSnapshot` when you need to separate
  retrieval from filesystem publication.
- Supply your own `ExportAPI` or `Store` implementation to change retrieval or
  storage. See [architecture](ARCHITECTURE.md) for component responsibilities.
- Use [mapping retrieval](MAPPINGS.md#library) to save Configuration Hub mappings
  separately from configuration snapshots.
- Custom backup integrations have additional
  [ownership and deletion requirements](API.md#custom-backup-integrations).
