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
| `make release-build VERSION=v0.7.0` | Require a clean checkout and matching tag at HEAD, run release checks, then build with that release version |
| `make toolchain` | Print the selected Go version |

Make targets select Go 1.27.3 through `GOTOOLCHAIN` and use read-only module mode so checks cannot update `go.mod` or `go.sum`. Formatting uses `gofmt` from that selected toolchain. The version and scanner pin live in the Makefile. For an intentional toolchain upgrade, update that pin along with `go.mod` and the build requirements; a one-run evaluation can use `make GO_TOOLCHAIN=go1.27.4 check`. Format newly added Go files before staging them; the formatting targets inspect Git-tracked files, including staged additions.

Test execution has a five-minute limit per package, configurable with `TEST_TIMEOUT`, for example `make TEST_TIMEOUT=10m test-race`. The race check needs a supported platform and a C compiler. Run filesystem validation on macOS with the storage requirements in [operations](OPERATIONS.md#snapshot-storage).

Before preparing a release, run:

```sh
make release-check
git diff --check
```

Vulnerability scanning requires network access to the Go vulnerability database and, on first use, the pinned scanner module. It prints the Go and scanner versions and reports reachable findings separately from package/module advisories. A reachable vulnerability or scanner failure fails the target; uncalled dependency advisories remain visible for review. Test and smoke checks leave source files unchanged; only `make fmt` reformats files, and only `make build` leaves a binary in the checkout.

Checks currently run locally. GitHub Actions workflow and macOS runner setup are tracked in [issue #7](https://github.com/easqr/iscconfigexporter/issues/7).

## Repository settings after migration

Settings were compared with the GitLab source on November 8, 2026. Both repositories are private and use `main` as the default branch. GitHub issues and pull requests are enabled, and linked issues close automatically after merge. Merge commits and squash merges are allowed; rebase merges are disabled. Squash commit messages use the pull request title, and merged head branches are deleted automatically. These are the supported equivalents of the source merge settings.

The source had no configured webhooks, active integrations, deploy keys, active deploy tokens, project or inherited CI/CD variables, pipeline trigger tokens, or secure files to migrate. No CI workflow was present; implementing GitHub Actions remains [issue #7](https://github.com/easqr/iscconfigexporter/issues/7).

The following differences still require an owner decision:

| Area | GitLab source | GitHub destination |
| --- | --- | --- |
| Branch protection | `main` permits pushes and merges by Maintainers; force pushes are disabled | GitHub reports that protection rules will not be enforced for this private repository until the organization upgrades to Team or Enterprise. No unenforced rule was created. |
| Membership | One inherited project Owner; no direct members | The organization has two members and a base Read role; only Owners can contribute. Existing organization membership and permissions were retained. |
| Wiki | Feature enabled | Private wikis require a plan upgrade. |
| Forking | Feature enabled for users with access | Private forking is disabled by the destination organization's policy. |
| Service Desk | Enabled, with confidential tickets by default | No direct GitHub repository equivalent was configured. Existing GitLab Service Desk remains in place. Support routing is tracked in [issue #8](https://github.com/easqr/iscconfigexporter/issues/8). |

GitLab remains available as the `gitlab` remote and retains the original issues and settings. It has not been archived or deleted. Migrated issue authors, creation dates and the original comment date are recorded in the issue text; GitHub's native timeline records the migration actions.

## Release identification

Use semantic version tags such as `v0.7.0`. `isc-exporter -version` works without policy or SDK configuration and reports the exporter version, full commit ID, modification state, compiler and platform. Metadata comes from Go's embedded build information; no Git command runs when the exporter starts. Missing commit or modification metadata is reported as `unknown`. Development builds without a module version report `development`.

Release builds inject the approved version through the linker. After committing and creating the matching tag at a clean checkout, run:

```sh
make release-build VERSION=v0.7.0
./isc-exporter -version
```

The target rejects missing or mismatched tags and modified or untracked files, runs release checks, and then builds the binary. It does not create, move or publish tags. `BUILD_OUTPUT` selects another output path, such as a temporary directory. For an ordinary local build, use `make build`; `make build VERSION=v0.7.0` injects a version without release enforcement and is useful only for testing build identification.

The linker setting is `-X github.com/easqr/iscconfigexporter/internal/buildinfo.releaseVersion=v0.7.0`. Without it, the exporter uses its embedded module version when available. VCS commit and modification state describe this module only when it is the main module. Builds made with VCS stamping disabled can still report their version, with unknown VCS fields.

SDK requests retain consumer identifier `isc-config-exporter`. A clean release uses consumer version `0.7.0`; development builds with a known commit use a shortened revision, for example `development+79976eb12345.dirty`. Modified builds carry a dirty marker. When imported as a library, the consumer version comes from the exporter dependency's module version, not the host application's version or commit. Replaced exporter dependencies report `development` because their original module version may not identify the code actually built.

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
