# Live-tenant validation checklist

Use this checklist to validate export, mapping retrieval, and recovery behavior against a nonproduction tenant.

## Preparation

Build the CLI using the [README](../README.md#build-and-run). Contributors should also run the separate [development checks](DEVELOPMENT.md#local-checks).

Use a nonproduction tenant and a private working directory. Confirm the required tenant role and API permissions with your administrator. SP-Config discovery and export require `sp:config:read`; backup inventory and mapping retrieval can require additional read permissions. An HTTP 403 requires reviewing authorization. The exporter does not broaden permissions.

1. Copy `exporter.example.json` to `exporter.json`. Set `tenant` to the identity returned in exports, choose a new private output directory, select supported types such as `ROLE` and `SOURCE`, and leave backup evaluation disabled.
2. Run `./isc-exporter -help` and `./isc-exporter -config exporter.json -validate`. Neither operation should need credentials or contact ISC.
3. Configure the test tenant's SDK `config.json` or `SAIL_*` settings using the [configuration reference](CONFIGURATION.md#native-sdk-connection-and-authentication). Protect credentials and confirm the connection targets the intended tenant.

## Export and publication

1. Run `./isc-exporter -config exporter.json > result.json`. Expect one asynchronous export job, a `COMPLETE` status and `<output>/<tenant>/manifest.json`. With backup evaluation disabled, `cleanup_error` should be absent. Keep results and logs private.
2. Resolve the tenant pointer once. Go applications can use `CurrentPath`. Check the manifest's tenant, job ID, counts and paths. Decode an `id-<hex>.json` basename and compare it with `self.id`; verify the type, version and complete envelope against the source export. Recalculate each artifact's SHA-256, for example with `shasum -a 256`, and compare it with the manifest.
3. Repeat without tenant changes. Expect a new generation and manifest; identical objects should appear under `changes.unchanged` with unchanged bytes, inode and modification time. Tenant-generated volatile fields can legitimately produce changes. If appropriate for the test tenant, change a test object, export again and check `changes.modified`; remove the test object and check `changes.deleted`.
4. Test include and exclude filters. The current managed snapshot should contain only the selected types; prior generations remain on disk. An unknown type or a selection that excludes all discovered types should fail before export initiation. A valid selection returning zero objects should publish an empty managed snapshot; test this only when the tenant can return such a result.

## Optional backup evaluation

With backup evaluation disabled, ordinary exports should make no Configuration Hub inventory requests. They do not issue separate backup-creation or upload requests.

If inventory access is available, set `backup.enabled=true`, `backup.policy="keep_latest"` and `backup.dry_run=true`, then export with `-cleanup-preview`. Expect unverified backups to be skipped and the ownership limitation to be reported. Optionally repeat with `backup.policy="older_than"` and `backup.age="7d"`.

The supplied SDK adapter cannot delete backups or populate the ownership registry automatically. A local registry does not remove that limitation. See [API capabilities and limits](API.md).

## Optional mapping retrieval

Follow the [mapping CLI example](MAPPINGS.md#cli), choosing a new directory separate from the export tree. Validate the request with `-validate`, then retrieve it. Confirm `mappings.json` and `provenance.json` exist, the source/target and API origin are correct, and the mapping checksum matches provenance. Configuration snapshots should remain unchanged.

## Cancellation and recovery

Send SIGINT during a test export. Cancellation before pointer replacement should preserve the previous generation; cancellation racing with replacement can still publish. The remote job may continue. SDK token acquisition cannot be interrupted by the export context; review [authentication limits](CONFIGURATION.md#authentication-limits) before interpreting cancellation results.

Test credential or permission failures only within your organization's lockout policy. Expect sanitized errors and no new published snapshot for failures before publication.

Inspect disk usage, stale locks and recovery using [operations](OPERATIONS.md). Verify the current generation before retrying a failed publication. Restoring a local pointer does not import configuration into ISC. Follow your credential-handling procedure when finished.
