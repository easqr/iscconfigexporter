# Documentation

Start with the [project README](../README.md) for purpose, build instructions and a first export.

## Usage and operations

- [Configuration](CONFIGURATION.md): exporter policy, SDK connection settings, authentication limits and retries.
- [Operations and troubleshooting](OPERATIONS.md): results, common errors, interrupted exports, locks and local rollback.
- [Snapshot files and readers](SNAPSHOTS.md): filenames, manifest fields, saved versions and Go reader modes.
- [Mapping retrieval](MAPPINGS.md): optional CLI and library usage, archive format and differ handoff.
- [Live-tenant validation](LIVE_VALIDATION.md): checklist for repeat operator verification.

## Technical review

- [Architecture](ARCHITECTURE.md): component responsibilities, artifact organization and publication guarantees.
- [API capabilities and limits](API.md): pinned SDK integration, backup ownership and deletion restrictions.
- [Dependencies and licenses](DEPENDENCIES.md): runtime inventory and retained notices.

## Development

- [Developer guide](DEVELOPMENT.md): local checks and dependency review.
- [SDK integration review](development/SDK_INTEGRATION.md): implementation details and automated verification.
- [Development history](development/HISTORY.md): historical commit and change index.
