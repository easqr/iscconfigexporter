# Configuration reference

Use this reference after the [first-export walkthrough](../README.md#build-and-run).
The exporter uses two separate configuration files:

| File | Purpose | Contains credentials? |
| --- | --- | --- |
| `exporter.json` | Tenant name, output directory, type filters, timing and backup evaluation | No |
| `config.json` | SDK connection URL and authentication settings | Yes |

Copy `exporter.example.json` to `exporter.json` and edit it for your tenant. Exporter policy must be a single JSON document with supported fields and field types; unknown fields and trailing content are rejected. SDK `SAIL_*` environment variables can replace `config.json`, as described [below](#native-sdk-connection-and-authentication).

## Exporter policy

Exporter policy means the export settings in `exporter.json`. The fields below
control what is exported and where it is saved. Defaults apply to omitted fields.
The example file explicitly sets `backup.dry_run` to `true` and `backup.registry` to `ownership.json`.

| Field | Default | Constraint/meaning |
|---|---|---|
| tenant | required | Required tenant name; must exactly match the downloaded export and follow the identifier rules below |
| output | exports | Directory for snapshots; use private local storage under your control |
| include | [] | Empty selects all discovered exportable types; otherwise explicit supported type names |
| exclude | [] | Supported types removed from selection; duplicates and include overlap rejected |
| timeout | 10m | Overall time limit for API requests, polling, snapshot saving and backup evaluation; token acquisition has the separate limit described below |
| poll_interval | 5s | Positive Go duration |
| retries | 3 | Additional transient read attempts, integer 0–10 |
| backoff | 1s | Positive Go duration, SDK exponential read backoff capped at 30s, except server Retry-After |
| log_level | info | debug, info, warn or error; no sensitive debug payload logging |
| backup.enabled | false | Enable read-only backup evaluation after an export; disabled by default |
| backup.policy | disabled | disabled, older_than, keep_latest; these settings do not enable deletion in the CLI |
| backup.age | empty | Required positive duration for older_than; e.g. 168h or 7d; a day is 24h; forbidden for keep_latest |
| backup.dry_run | false | Prevent deletion in a custom backup integration; the supplied CLI integration never deletes |
| backup.registry | empty | Optional path to local ownership records for a custom integration; records alone do not enable deletion |

`backup.enabled=true` requires `backup.policy` to be `older_than` or `keep_latest`. You can configure a policy while leaving evaluation disabled, but its fields must still be valid. Type filters are checked against the tenant when an export runs; local validation cannot verify which types it supports. If the filters leave no types selected, no export job is started. Durations accept Go units such as `30s` and `10m`, plus positive whole days such as `7d`.

### Choose export types

Set `include` to a list of type names to export only those types:

```json
{
  "tenant": "example",
  "output": "exports",
  "include": ["ROLE", "SOURCE"]
}
```

Replace `example` with your tenant's identity. Type names must be supported by
that tenant. Use `"include": []` to start with all discovered types, and use
`exclude` to remove selected types. Do not put the same type in both lists.
Changing filters replaces the current snapshot's managed inventory; previously
saved generations remain on disk.

## Identifier rules

Tenant and type names contain 1–100 ASCII letters (`A–Z`, `a–z`), digits, hyphens or underscores, and start with a letter or digit. Names such as `CON`, `PRN`, `AUX`, `NUL`, `COM1` through `COM9`, and `LPT1` through `LPT9` are reserved and cannot be used, regardless of letter case. These restrictions keep directory names usable on different filesystems.

Type names must match the tenant's supported export types exactly, including letter case. A name can satisfy the character rules and still be unsupported by that tenant.

## Native SDK connection and authentication

Connection and authentication settings use the SailPoint SDK format. The exporter loads these settings from `config.json` or `SAIL_*` environment variables; the SDK handles authentication and token caching. Go applications can supply a `*sailpoint.Configuration` through `sdk.NewWithConfiguration`. Caller-owned configuration and HTTP clients are copied rather than modified.

Use an existing SDK `config.json`, or create it manually with the following fields. If the separate **SailPoint CLI** is installed, its optional `sail sdk init config` command can generate the file instead. The SailPoint CLI is not required to run the exporter.

```json
{
  "ClientId": "<client-id>",
  "ClientSecret": "<client-secret>",
  "BaseURL": "https://<tenant>.api.identitynow.com"
}
```

This file contains secrets and is ignored by Git. The exporter never creates or rewrites it. Protect it with local filesystem permissions. As an alternative to the file, supply these SDK environment variables:

| Variable | Value |
| --- | --- |
| `SAIL_BASE_URL` | Your tenant's HTTPS API URL |
| `SAIL_CLIENT_ID` | SDK client ID |
| `SAIL_CLIENT_SECRET` | SDK client secret |

Connection lookup order:

1. If `SAIL_BASE_URL` is set, use the `SAIL_*` environment configuration as a whole (it does not merge missing credentials from files).
2. Otherwise look for exactly `config.json` beside the executable, then in the working directory. Other filenames and extensions are ignored. A malformed or unreadable selected file returns an error rather than falling through to another file.
3. Otherwise return a configuration error. Home-directory configuration is not read.

Missing or malformed configuration produces an error without exposing file contents. `-help` and local `-validate` do not load SDK settings, access credentials, or contact ISC.

## Authentication limits

SDK v2.7.106 token requests use a separate HTTP client. They cannot be interrupted by the export context or configured API timeout, and do not use the API transport override or redirect policy. Token URLs must use HTTPS without user information or a query string.

Cached tokens are not refreshed by expiry; recreate a long-running application's adapter after an expired-token failure. Token transport errors can appear on standard output, so scripts expecting JSON must account for them. Programmatically supplied SDK tokens are supported.

## Request retries

The `retries` setting controls additional attempts; `backoff` controls the initial delay, capped at 30 seconds. Only GET responses with HTTP 408, 429 or 5xx may retry; POST, unclassified transport/auth failures and permanent errors do not. Delays grow exponentially, up to 30 seconds. A server `Retry-After` value on HTTP 429 or 503 can exceed that cap, but waits remain bounded by the request context.

## Backup evaluation

**The supplied CLI integration is read-only.** It can list Configuration Hub backups and report an ownership limitation, but cannot establish which backups belong to this exporter. It skips unverified records and cannot delete backups. Enabling evaluation, choosing a policy, setting `backup.dry_run=false`, or providing a registry does not change that limit.

`backup.enabled` is `false` by default. When enabled with a valid policy, evaluation runs after a successful export and requires the appropriate Configuration Hub read access. `-cleanup-preview` forces preview behavior for an enabled policy; it still runs an export and saves a snapshot. With evaluation disabled, that option does not enable it.

The result uses the JSON field names `cleanup` and `cleanup_error` for backup evaluation. See [results and errors](OPERATIONS.md#results-and-errors) for their meaning.

### Retention in custom Go integrations

Go applications can supply a separately verified backup integration. In that case, `keep_latest` protects every successful exporter-owned backup tied for newest. `older_than` selects backups created strictly before the age cutoff; a backup at the exact boundary is retained. Both require independently verified ownership, complete nonpartial status, explicit deletion permission and a surviving protected successor.

These retention guarantees describe the library's custom-provider path, not deletion capability in the supplied CLI. See [custom backup integrations](API.md#custom-backup-integrations) for the required ownership and deletion guarantees.
