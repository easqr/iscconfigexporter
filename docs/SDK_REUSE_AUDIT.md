# SDK reuse audit

Scope: all production Go files (root package, sdk and cmd), configuration examples, direct dependencies and documented operational behavior. Compared with pinned SailPoint Go SDK v2.7.106 configuration.go, client.go, paginator.go, api_beta/api_sp_config.go, generated export models and api_v2026/api_configuration_hub.go. Also inspected v3.1.74 configuration.go and SP-Config client authentication to check whether a major upgrade would remove native-auth limitations. No live API calls were made.

## Removed duplication

| Previous exporter behavior | SDK-native replacement |
|---|---|
| Credentials/CredentialsFromEnv reading ISC_CLIENT_ID and ISC_CLIENT_SECRET | NewDefaultConfiguration reads the SDK's own sources; explicit native Configuration also accepted |
| Exporter BaseURL and manual `/oauth/token` construction | SDK ClientConfiguration.BaseURL/TokenURL, supplied by native discovery or embedded caller |
| OAuth clientcredentials token-source construction on each API operation | Generated SDK default authentication and token cache |
| Separate beta/v2026 configurations and HTTP clients | One root NewAPIClient provides both versioned API services |
| Engine.read retry loop, custom transient interface and exponential backoff | SDK-supported retryable HTTP client, its retry loop, DefaultBackoff and Retry-After parsing |

The retry policy still restricts eligibility to transient GET responses and forbids POST retries. This is exporter safety configuration of the SDK mechanism, not a second retry implementation. The API adapter does not perform HTTP requests directly or implement OAuth protocol handling.

## Reviewed responsibilities retained

| Code | Why exporter logic remains necessary |
|---|---|
| engine.go, service.go | SDK methods issue individual requests; no composed discovery/select/start/poll/download/publish/cleanup workflow or publication gate exists. Job polling is not an HTTP retry. Context/deadline management and SDK-independent API/Store/Lifecycle interfaces belong to the component. |
| config.go, CLI | Export filters, output, timeouts, logging and retention policies are exporter options, not SDK connection settings. SDK config.json is distinct from exporter.json. Strict policy validation is retained; no credential loader exists in production exporter code. |
| SDK adapter selection/error handling | SDK exposes Exportable and IncludeTypes but does not validate an entire requested set against tenant discovery, reject empty selection, sanitize sensitive error bodies, enforce HTTPS origins, forbid export POST retries or turn default-loader panics into sanitized component errors. Those boundaries remain. |
| SDK raw download handling | SDK restores raw response bytes, which the adapter consumes rather than reserializing a generated model that omits unknown envelope/signature fields. This reuses SDK request construction/response handling and preserves original data; no parallel REST adapter exists. |
| organize.go | SDK model serialization does not provide full unknown-envelope preservation, duplicate JSON-key rejection, lossless numeric handling, portable stable object paths, identity/collision validation, artifact checksums or a filesystem manifest. Standard encoding/json remains the serializer; the small decoder layer enforces rejection of duplicate keys. |
| filesystem.go | Staging, inode reuse, managed-only reconciliation, atomic generation publication, fsync, reader pinning and interruption recovery are not SDK capabilities. |
| backup.go, registry.go | SDK inventory/deletion methods do not establish exporter ownership, persist provenance, evaluate retention, protect the latest backup, preview deletion, recheck eligibility or isolate cleanup errors from export success. These are essential safeguards. |
| sdk/backups.go Get | The pinned SDK exposes ListBackups and DeleteBackup for ordinary backups, but no ordinary-backup Get endpoint. The adapter enumerates and validates an exact unique ID; uploaded-configuration Get is a different resource and must not be substituted. SDK destructive deletion remains blocked because safe ownership cannot be verified. |
| Pagination | The SDK's generic paginator requires Limit/Offset methods. Neither SP-Config discovery/download nor Hub ListBackups in this version offers those request methods. No exporter paginator is implemented, and applying the SDK reflection paginator would be invalid. |
| Adapter gate/default-loader lock | Generated clients mutate cfg.Token without synchronization; the SDK loader uses global Viper state. Serialization protects adapter calls and its own configuration loads. Embedded applications sharing Viper should provide explicit SDK configuration. This does not add a token cache. |

No other redundant SDK protocol, credential, token-source, retry-loop, pagination, endpoint construction or model reimplementation was found in the reviewed scope. Exporter domain contracts intentionally remain independent of generated models.

## Upstream constraints retained and documented

The native SDK token helper creates its own unbounded http.Client, does not take the API context or configured transport, does not apply the API redirect policy, may print transport errors to stdout, and caches tokens without expiry-based refresh. Default configuration may print a deprecated-home-config warning and panic on discovery failures. The adapter sanitizes discovery panics and serializes token-cache access; it does not replace the native authentication implementation to mask these limitations. The same helper limitations were present in inspected v3.1.74, so a major upgrade would not solve them. See CONFIGURATION.md for exact operator implications.

## Verification

Tests cover native config.json loading, optional native SAIL_* loading and precedence, missing/malformed/incomplete configuration, ignored legacy ISC_* variables, explicit native configuration (including existing token), caller configuration preservation, native token acquisition/caching, concurrent adapter use, SDK GET retries/backoff/Retry-After, permanent failures, no POST retry, cancellation, real SDK fake-transport end-to-end export/Hub inventory, and CLI help/local validation without SDK settings. Existing object, filesystem, retention and external-consumer tests continue to run. Tests intercept SDK token transport separately and open no network connections.
