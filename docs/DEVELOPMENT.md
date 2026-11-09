# Developer guide

This guide is for contributors. For building and using the exporter, start with the [README](../README.md). The [documentation index](README.md) separates usage, technical review and development references.

## Local checks

Use Go 1.27.3 or newer, including for release builds. Go 1.27.3 fixes the certificate-validation vulnerability [GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809). Check `go version` before building; updating `go.mod` does not patch an already built binary.

Run the shared checks from the repository root:

```sh
make check
```

`make check` checks formatting without editing files, runs vet, ordinary tests and race tests, then builds a temporary CLI and checks its help and example-policy validation. These checks do not contact ISC or need credentials. Dependencies and the pinned Go toolchain must already be available for fully offline use; otherwise Go may download them. SDK integration tests use fake HTTP transports; passing them does not verify live tenant behavior.

The Make targets are also available individually:

| Target | Behavior |
| --- | --- |
| `make fmt` | Format tracked Go files; this is the explicit editing command |
| `make fmt-check` | List unformatted tracked Go files and fail without modifying them |
| `make vet` | Run `go vet ./...` |
| `make test` | Run the ordinary test suite |
| `make test-race` | Run the test suite with the race detector |
| `make smoke` | Build a temporary CLI, run `-help` and validate `exporter.example.json`; remove the temporary build afterward |
| `make vuln` | Run pinned govulncheck v1.9.0 against the current vulnerability database |
| `make release-check` | Run `make check` and vulnerability scanning |
| `make build` | Build `isc-exporter` in the repository root |
| `make toolchain` | Print the selected Go version |

Make targets select Go 1.27.3 through `GOTOOLCHAIN` and use read-only module mode so checks cannot update `go.mod` or `go.sum`. Formatting uses `gofmt` from that selected toolchain. The version and scanner pin live in the Makefile. For an intentional toolchain upgrade, update that pin along with `go.mod` and the build requirements; a one-run evaluation can use `make GO_TOOLCHAIN=go1.27.4 check`. Format newly added Go files before staging them; the formatting targets inspect Git-tracked files, including staged additions.

Test execution has a five-minute limit per package, configurable with `TEST_TIMEOUT`, for example `make TEST_TIMEOUT=10m test-race`. The race check needs a supported platform and a C compiler. Run filesystem validation on macOS with the storage requirements in [operations](OPERATIONS.md#snapshot-storage).

Before preparing a release, run:

```sh
make release-check
git diff --check
```

Vulnerability scanning requires network access to the Go vulnerability database and, on first use, the pinned scanner module. It prints the Go and scanner versions and reports reachable findings separately from package/module advisories. A reachable vulnerability or scanner failure fails the target; uncalled dependency advisories remain visible for review. Test and smoke checks leave source files unchanged; only `make fmt` reformats files, and only `make build` leaves a binary in the checkout.

Checks currently run locally. GitLab CI workflow and macOS runner setup are tracked in [issue #7](https://gitlab.com/easqrtech/iam/iscconfigexporter/-/work_items/7).

See the [SDK integration review](development/SDK_INTEGRATION.md#automated-verification) for coverage and the [live-tenant checklist](LIVE_VALIDATION.md) for operator checks. The owner has reported completing live verification; no individual live test results are recorded in this repository's checklist.

GoDoc includes runnable offline examples for `Service.Export`, `RetrieveMappings`,
`ParseDuration`, `snapshot.Read` and `snapshot.FromFiles`. They use local providers
and temporary files, without SDK credentials or network access. Run them with:

```sh
go test -run '^Example' ./...
```

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
