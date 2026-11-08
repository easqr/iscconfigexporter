# Development and review

This guide is for contributors and coding agents. For building and using the exporter, start with the [README](../README.md).

## Local checks

Run from the repository root:

```sh
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
```

`make check` runs formatting, vet and the ordinary test suite; run the race check separately. Tests require no tenant credentials or external services. SDK integration tests use fake HTTP transports; passing them does not verify live tenant behavior.

For CLI changes, also build the binary, check `-help`, and validate the example policy without contacting a tenant:

```sh
go build -o isc-exporter ./cmd/isc-exporter
./isc-exporter -help
./isc-exporter -config exporter.example.json -validate
git diff --check
```

## Existing review records

- [Architecture](ARCHITECTURE.md): component boundaries, publication guarantees and retained implementation responsibilities.
- [SDK reuse audit](SDK_REUSE_AUDIT.md): native SDK integration decisions, upstream constraints and verification coverage.
- [API findings](API.md): backup ownership and conditional deletion limitations.
- [Implementation progress](../PROGRESS.md): historical development slices and validation records. Statements about repository state and live validation describe the time of those entries.
- [Live-tenant validation checklist](LIVE_VALIDATION.md): operator checks for tenant behavior. The owner has reported completing live-tenant verification; historical offline test records remain unchanged.
