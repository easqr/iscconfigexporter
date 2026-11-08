# Developer guide

This guide is for contributors. For building and using the exporter, start with the [README](../README.md). The [documentation index](README.md) separates usage, technical review and development references.

## Local checks

Run from the repository root:

```sh
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
```

`make check` runs formatting, vet and the ordinary test suite; run the race check separately. Automated tests require no tenant credentials or external services. SDK integration tests use fake HTTP transports; passing them does not verify live tenant behavior.

For CLI changes, also build the binary, check `-help`, and validate the example policy without contacting a tenant:

```sh
go build -o isc-exporter ./cmd/isc-exporter
./isc-exporter -help
./isc-exporter -config exporter.example.json -validate
git diff --check
```

See the [SDK integration review](development/SDK_INTEGRATION.md#automated-verification) for coverage and the [live-tenant checklist](LIVE_VALIDATION.md) for operator checks. The owner has reported completing live verification; no individual live test results are recorded in this repository's checklist.

## Dependency review

When imports or dependency versions change, inspect the CLI's actual runtime modules:

```sh
go list -deps -f '{{with .Module}}{{.Path}} {{.Version}}{{end}}' ./cmd/isc-exporter
```

Deduplicate the output, exclude the exporter module, and compare it with [dependencies and licenses](DEPENDENCIES.md). Review the relevant module license files and maintain the full notices under `docs/licenses`. A module in the SDK's dependency graph is not necessarily linked into the CLI. Keep runtime dependencies and test-only notices distinct.

SDK updates also require checking endpoint contracts, raw response preservation, authentication behavior, token caches, retries and license notices. Do not infer safe backup deletion from matching IDs or a local ownership record; see [API limits](API.md#custom-backup-integrations).

## Design and history

- [Architecture](ARCHITECTURE.md): public component boundaries and publication guarantees.
- [SDK integration review](development/SDK_INTEGRATION.md): current adapter responsibilities, upstream constraints and test coverage.
- [Development history](development/HISTORY.md): historical implementation and change index.
