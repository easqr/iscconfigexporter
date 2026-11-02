# Configuration reference

Configuration is one strict JSON document; unknown fields/trailing documents are rejected. JSON avoids adding another configuration parser. Start with config.example.json. Credentials are never accepted in this file.

| Field | Default | Constraint/meaning |
|---|---|---|
| tenant | required | Portable ASCII identifier, max 100 characters, no Windows device names; must exactly match download tenant |
| base_url | required | HTTPS origin, no userinfo, query, fragment or path except `/`; operator supplies trusted API tenant host |
| output | exports | Nonempty private trusted output root |
| include | [] | Empty selects all discovered exportable types; otherwise explicit supported type names |
| exclude | [] | Supported types removed from selection; duplicates and include overlap rejected |
| timeout | 10m | Positive Go duration; Service bounds full export, publication and cleanup |
| poll_interval | 5s | Positive Go duration |
| retries | 3 | Additional transient read attempts, integer 0–10 |
| backoff | 1s | Positive Go duration, exponential read backoff capped at 30s after first delay |
| log_level | info | debug, info, warn or error; no sensitive debug payload logging |
| backup.enabled | false | Cleanup disabled unless true |
| backup.policy | disabled | disabled, older_than, keep_latest |
| backup.age | empty | Required positive duration for older_than; e.g. 168h or 7d; a day is 24h; forbidden for keep_latest |
| backup.dry_run | false | Candidate evaluation only; no delete requests |
| backup.registry | empty | Optional local trusted registry path; example uses ownership.json |

Enabled=true with policy=disabled is invalid. A disabled cleanup with a configured retention policy is allowed as a staged configuration, but the policy must still be valid. Types are validated against discovery at execution; local validation checks syntax without network or credentials. An empty final selection fails before POST. Hours/days are supported by Duration; standard Go units also work. Days are positive integer days with overflow bounds.

Authentication uses ISC_CLIENT_ID and ISC_CLIENT_SECRET and the official SDK's OAuth2 token-source context mechanism. The adapter supports client credentials only; it derives `/oauth/token` from base_url. No token persistence, SDK config-file lookup or global credential state is used. Token requests share the caller's cancellation/deadline, use a bounded HTTP client, refresh through a request-scoped token source, and never redirect. Each API operation acquires its own token source to avoid sharing SDK mutable token state.

Keep-latest protects every equally newest successful exporter-owned backup. Older-than deletes strictly earlier than now-age; exact boundary is retained. Both require independent verified ownership, successful nonpartial state, explicit deletion permission and a surviving protected successor. The shipped SDK adapter is inventory-only, regardless of these settings; it reports ownership limitations instead of deleting.
