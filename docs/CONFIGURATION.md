# Configuration reference

Exporter policy is a single JSON document. Unknown fields and trailing documents are rejected. Copy `exporter.example.json` to `exporter.json` and edit it for your tenant. Keep connection settings and credentials in the separate SDK `config.json` file.

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

Enabled=true with policy=disabled is invalid. A disabled cleanup with a configured retention policy is allowed as a staged configuration, but the policy must still be valid. Types are validated against discovery at execution; local validation checks syntax without network or credentials. An empty final selection fails before POST. Durations accept Go units such as `30s` and `10m`, plus positive whole days such as `7d`.

## Native SDK connection and authentication

Connection and authentication settings use the SailPoint SDK format. The exporter loads these settings from `config.json` or `SAIL_*` environment variables; the SDK handles authentication and token caching. Go applications can supply a `*sailpoint.Configuration` through `sdk.NewWithConfiguration`. Caller-owned configuration and HTTP clients are copied rather than modified.

Use your existing SDK configuration. The normal file is `config.json`, generated if desired with `sail sdk init config`. For example:

```json
{
  "ClientId": "<client-id>",
  "ClientSecret": "<client-secret>",
  "BaseURL": "https://<tenant>.api.identitynow.com"
}
```

This file contains secrets and is ignored by Git. The exporter never creates or rewrites it. Protect it with local filesystem permissions. The adapter also accepts the SDK environment variables SAIL_BASE_URL, SAIL_CLIENT_ID and SAIL_CLIENT_SECRET as an optional alternative.

Connection lookup order:

1. If SAIL_BASE_URL is set, use the SAIL_* environment configuration as a whole (it does not merge missing credentials from files).
2. Otherwise look for exactly `config.json` beside the executable, then in the working directory. Other filenames and extensions are ignored. A malformed or unreadable selected file returns an error rather than falling through to another file.
3. Otherwise return a configuration error. Home-directory configuration is not read.

Missing or malformed configuration produces an error without exposing file contents. `-help` and local `-validate` do not load SDK settings, access credentials, or contact ISC.

## Authentication limits

The pinned SDK (v2.7.106) acquires tokens on demand and caches them without expiry-based refresh. A blocked token request cannot be interrupted by the export timeout or cancellation signal: token acquisition uses a separate HTTP client, outside the configured API timeout, transport and redirect policy. Token URLs must use HTTPS without user information or a query string, but token-request redirects follow the SDK's behavior.

The SDK may print token transport errors to standard output. This can affect scripts expecting JSON output. If a long-running Go application encounters an expired-token failure, recreate its adapter; automatic reauthentication is not provided. Programmatically supplied SDK tokens are also supported.

## Request retries

The `retries` setting controls additional attempts; `backoff` controls the initial delay, capped at 30 seconds. Only GET responses with HTTP 408, 429 or 5xx may retry; POST, unclassified transport/auth failures and permanent errors do not. Delays grow exponentially, up to 30 seconds. A server `Retry-After` value on HTTP 429 or 503 can exceed that cap, but waits remain bounded by the request context.

## Backup retention

Keep-latest protects every equally newest successful exporter-owned backup. Older-than selects backups created strictly before the age cutoff; a backup at the exact boundary is retained. Both require independent verified ownership, successful nonpartial state, explicit deletion permission and a surviving protected successor. The supplied SDK adapter is inventory-only, regardless of these settings; it reports ownership limitations instead of deleting. See [API capabilities and limits](API.md) for details.
