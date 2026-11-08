# SDK integration review

This guide describes the current integration with SailPoint Go SDK v2.7.106 for contributors and technical reviewers. Operational settings and limitations belong in the [configuration reference](../CONFIGURATION.md); public component responsibilities are in [architecture](../ARCHITECTURE.md).

## Configuration boundary

`sdk.New` validates exporter policy, then loads connection settings. `SAIL_BASE_URL` selects environment configuration as a whole; otherwise the loader selects exactly `config.json` beside the executable, then in the working directory. It uses a private Viper instance and `SetConfigFile`, avoiding global state and broad filename discovery. Missing or malformed settings produce sanitized errors.

The loader constructs `sailpoint.ClientConfiguration`, sets the token URL from the base URL, and calls `sailpoint.NewConfiguration`. It does not call `NewDefaultConfiguration` or read deprecated home-directory configuration. `sdk.NewWithConfiguration` accepts explicit SDK configuration for embedded applications. Caller-owned configuration, headers and HTTP client settings are copied before adapter changes.

Authentication and token caching remain SDK responsibilities. One root `NewAPIClient` provides the Beta export and V2026 backup/mapping clients. Generated clients maintain separate per-version token caches. A context-aware gate serializes calls through an adapter because those caches are mutable. There is no exporter token cache or OAuth exchange implementation.

## Requests and retries

The SDK constructs and executes API requests. The adapter supplies its retryable HTTP client with the policy's retry count and backoff, a default API timeout, a GET-only transient-response retry policy and disabled API redirects. SDK `DefaultBackoff` handles exponential delays and `Retry-After`. Export initiation is never retried.

`Engine` handles job polling. The pinned Go SDK exposes job status as a string, with no enum constants. The engine accepts the five [documented export states](https://developer.sailpoint.com/docs/tools/sdk/powershell/beta/models/sp-config-export-job-status/): `NOT_STARTED`, `IN_PROGRESS`, `COMPLETE`, `CANCELLED` and `FAILED`. Other values are errors.

The SDK restores raw response bytes after generated decoding. The export and mapping adapters read those bytes rather than reserializing generated models, preserving unknown fields and numeric precision. SDK decoder requirements still apply before the raw body is consumed. API errors with response bodies are sanitized.

## Exporter responsibilities

| Component | Responsibility |
| --- | --- |
| `config.go`, CLI | Export policy, strict policy decoding, option validation and command composition |
| `engine.go`, `service.go` | Type discovery and selection, one export initiation, polling, deadlines, organization, publication and post-publication backup evaluation |
| `organize.go`, `snapshot` | Complete-envelope validation, duplicate-key rejection, lossless numbers, canonical paths, checksums, snapshot models and readers |
| `filesystem.go` | Generation staging, unchanged-file hardlinks, reconciliation, locking, sync and atomic pointer publication |
| `backup.go`, `registry.go` | Retention decisions, trusted local provenance, ownership checks and cleanup reporting |
| `sdk/backups.go` | Read-only Hub inventory; `Get` finds a unique record by listing because the pinned client has no ordinary-backup Get method; `Delete` is blocked |
| `mappings.go`, `mapping_filesystem.go` | Mapping-byte validation, retrieval provenance and separate archive publication |

Snapshot models live in the `snapshot` package. Root code uses those models and the shared decoder, checksum, object-path and managed-path functions directly. No root-package compatibility aliases remain.

SP-Config discovery and download, Hub backup listing and mapping retrieval expose no limit/offset request controls in this SDK. The generic SDK paginator requires those methods and cannot be applied to these requests.

## Upstream constraints

Native token acquisition uses a separate SDK-created HTTP client without the export context, configured API transport or API timeout. It may print transport errors to standard output, follows its own redirect behavior, and caches tokens without expiry-based refresh. Adapter serialization protects concurrent cache access but does not change those behaviors. See [authentication limits](../CONFIGURATION.md#authentication-limits) for operator implications.

The supplied backup adapter cannot establish export-to-backup ownership or conditional deletion eligibility. See [API capabilities and limits](../API.md) before changing that boundary. An ownership verifier that merely trusts a local registry does not resolve it.

## Automated verification

The offline suite covers:

- Exact configuration filenames, file/environment precedence, missing or malformed settings, and rejection of legacy settings.
- Explicit SDK configuration, caller-owned settings, native token acquisition/caching and concurrent adapter calls.
- Generated export and Hub requests through fake transports, GET retries and `Retry-After`, permanent failures, no POST retries and cancellation.
- Complete object envelopes, unknown fields, numeric precision, unsafe or duplicate identities and deterministic artifact paths.
- Snapshot readers, repeated publication, reconciliation, concurrent writers and injected publication failures.
- Retention policies and destructive-provider checks through local doubles; the supplied SDK adapter issues no DELETE requests.
- Mapping response preservation, source groups, strict validation, provenance, private file permissions, output separation, no replacement and CLI operation isolation.
- External-package library usage, CLI help and credential-free local validation. The sibling differ also has an offline mapping-bundle handoff test.

Tests use local doubles and no live tenant credentials. Owner-run live verification is separate; see the [validation checklist](../LIVE_VALIDATION.md).
