# Configuration reference

Configuration is one strict JSON document; unknown fields/trailing documents are rejected. JSON avoids adding another configuration parser. Start with exporter.example.json and save policy as exporter.json. Credentials are never accepted in this file; config.json is reserved for the SDK.

| Field | Default | Constraint/meaning |
|---|---|---|
| tenant | required | Portable ASCII identifier, max 100 characters, no Windows device names; must exactly match download tenant |
| output | exports | Nonempty private trusted output root |
| include | [] | Empty selects all discovered exportable types; otherwise explicit supported type names |
| exclude | [] | Supported types removed from selection; duplicates and include overlap rejected |
| timeout | 10m | Positive Go duration; Service bounds API requests, polling, publication and cleanup; native token acquisition has an upstream limitation below |
| poll_interval | 5s | Positive Go duration |
| retries | 3 | Additional transient read attempts, integer 0–10 |
| backoff | 1s | Positive Go duration, SDK exponential read backoff capped at 30s, except server Retry-After |
| log_level | info | debug, info, warn or error; no sensitive debug payload logging |
| backup.enabled | false | Cleanup disabled unless true |
| backup.policy | disabled | disabled, older_than, keep_latest |
| backup.age | empty | Required positive duration for older_than; e.g. 168h or 7d; a day is 24h; forbidden for keep_latest |
| backup.dry_run | false | Candidate evaluation only; no delete requests |
| backup.registry | empty | Optional local trusted registry path; example uses ownership.json |

Enabled=true with policy=disabled is invalid. A disabled cleanup with a configured retention policy is allowed as a staged configuration, but the policy must still be valid. Types are validated against discovery at execution; local validation checks syntax without network or credentials. An empty final selection fails before POST. Hours/days are supported by Duration; standard Go units also work. Days are positive integer days with overflow bounds.

## Native SDK connection and authentication

The adapter loads SDK connection settings from local `config.json` or `SAIL_*` environment variables, then calls `sailpoint.NewConfiguration()` and `sailpoint.NewAPIClient()` from pinned SDK v2.7.106. Authentication, token exchange and caching remain SDK responsibilities. Both SP-Config and Configuration Hub use the same root SDK client. Optional programmatic configuration is accepted by sdk.NewWithConfiguration using the SDK's own Configuration type; caller-owned configuration and HTTP clients are copied rather than mutated.

Use your existing SDK configuration. The normal file is `config.json`, generated if desired with `sail sdk init config`. Its placeholder-only shape is:

```json
{
  "ClientId": "<client-id>",
  "ClientSecret": "<client-secret>",
  "BaseURL": "https://<tenant>.api.identitynow.com"
}
```

This file contains secrets and is ignored by Git. The exporter never creates or rewrites it. Protect it with local filesystem permissions. No secret manager is required. The adapter also accepts the SDK-compatible SAIL_BASE_URL, SAIL_CLIENT_ID and SAIL_CLIENT_SECRET environment variables as an optional alternative.

Connection lookup order:

1. If SAIL_BASE_URL is set, use the SAIL_* environment configuration as a whole (it does not merge missing credentials from files).
2. Otherwise search for config.json beside the executable, then in the working directory.
3. Otherwise return a configuration error. The deprecated `~/.sailpoint/config.yaml` is not read.

Missing or malformed configuration is reported as a sanitized error. Configuration loading uses a private parser and does not share the SDK's global Viper state. Help and local -validate do not load SDK configuration or access credentials/network.

Native SDK limitations verified in v2.7.106: generated clients lazily acquire and cache tokens, without expiry-based refresh; acquisition uses an SDK-created http.Client rather than the configured API client, and does not receive the request context, timeout or redirect policy. Therefore the export timeout bounds API requests and retry waits but cannot interrupt a blocked native token request. API transport injection likewise does not intercept native token acquisition. Token endpoints are validated as HTTPS URLs without query/userinfo, but the SDK's token HTTP redirect behavior is not overridden. The SDK may print token transport errors to stdout. Recreate an adapter after an expired-token failure; automatic reauthentication is not added by the exporter. Programmatic native Token configuration is also supported. Calls sharing an adapter are serialized to protect generated SDK mutable token caches. These limitations also exist in inspected SDK v3.1.74; no API-version migration was introduced for this change.

The exporter configures the SDK's existing retryable HTTP client with RetryMax=retries and minimum delay=backoff (clamped to 30s). Only GET responses with HTTP 408, 429 or 5xx may retry; POST, unclassified transport/auth failures and permanent errors do not. SDK DefaultBackoff supplies exponential delay (30s cap) and honors Retry-After on 429/503, which can exceed that cap but remains bounded by the request context. There is no second engine retry loop.


Keep-latest protects every equally newest successful exporter-owned backup. Older-than deletes strictly earlier than now-age; exact boundary is retained. Both require independent verified ownership, successful nonpartial state, explicit deletion permission and a surviving protected successor. The shipped SDK adapter is inventory-only, regardless of these settings; it reports ownership limitations instead of deleting.
