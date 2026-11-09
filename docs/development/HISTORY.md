# Development history

This is a historical index for contributors. Current usage and behavior are documented in the [README](../../README.md), [configuration](../CONFIGURATION.md), [operations](../OPERATIONS.md) and [architecture](../ARCHITECTURE.md). Git history contains the detailed implementation records.

## Initial implementation

| Area | Commit | Work recorded |
| --- | --- | --- |
| Foundation | `460e855` | Configuration, public contracts, pinned SDK and local CLI validation |
| SDK integration | `3506d9a` | Type discovery, filtering, export initiation, polling and fake-transport verification |
| Object organization | `5e9b643` | Complete envelopes, unknown fields, lossless numbers, portable paths and checksums |
| Filesystem publication | `5122be7` | Staged generations, atomic tenant pointer, unchanged-file hardlinks, reconciliation and locking |
| Backup lifecycle | `f7b1028` | Retention policies, registry and ownership safeguards; SDK inventory with deletion blocked |
| Integration | `6ddc24f` | Offline integration and failure tests, race/static checks and operational documentation |

These initial checks ran on Go 1.27.2, macOS arm64, using local doubles.

## Subsequent changes

- Connection and authentication settings adopted the SDK format, separating SDK `config.json` from exporter policy `exporter.json`. Token exchange and caching use the native SDK. HTTP retries use the SDK's retryable client.
- Snapshot models, decoding and reader contracts were extracted into the SDK-free `snapshot` package.
- Optional read-only mapping retrieval added separate checksum-bound mapping archives.
- The module path became `gitlab.com/easqrtech/iam/iscconfigexporter`.
- `46ffcab` removed the deprecated SDK home-directory configuration fallback.
- `701f210` removed root snapshot aliases and internal forwarding helpers, consolidated managed-path validation, restricted discovery to exactly `config.json`, and aligned polling with documented export states.
- The repository and module moved to `github.com/easqr/iscconfigexporter`. Tags `v0.6.0` and `v0.7.0` retain their GitLab module path; `v0.8.0` uses the GitHub module path.
