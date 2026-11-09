# API capabilities and limits

The exporter uses SailPoint Go SDK **v2.7.106**. The endpoint descriptions below reflect that pinned dependency, rather than a claim about the latest SDK release. Live-tenant verification has been completed by the owner; the [validation checklist](LIVE_VALIDATION.md) is available for repeat checks.

## Configuration exports

The Beta SP-Config client provides discovery (`ListSpConfigObjects`), export initiation (`ExportSpConfig` with `IncludeTypes`), status polling (`GetSpConfigExportStatus`) and download (`GetSpConfigExport`). These methods expose no pagination controls. The complete download is buffered in memory, so memory requirements grow with export size; see [memory investigation #5](https://github.com/easqr/iscconfigexporter/issues/5). Results expire; download them promptly after completion.

The supported job states are `NOT_STARTED`, `IN_PROGRESS`, `COMPLETE`, `CANCELLED` and `FAILED`. Only `COMPLETE` permits download. The pinned Go SDK represents status as a string; the exporter rejects unrecognized values.

The adapter reads the SDK's restored raw response body after decoding. This preserves envelope fields, signatures and numeric precision that could be lost by serializing generated models. It does not verify signatures or redact tenant configuration.

Connection settings, authentication limits and retries are described in the [configuration reference](CONFIGURATION.md). Component boundaries are described in [architecture](ARCHITECTURE.md).

## Configuration Hub backup inventory

The V2026 `ListBackups` method returns an array of backup records. It exposes status filtering, but no pagination controls. Records include job ID, tenant, status, creation time, deletion permission and partial-backup status. Uploaded configurations are distinct resources with separate endpoints.

The reviewed SDK contract does not establish that an SP-Config export job ID identifies a persistent Configuration Hub backup, or that every export creates a deletable backup. Names, requesters, types and matching IDs are insufficient evidence of exporter ownership. The exporter does not create extra backups, upload configurations or infer registry records from export responses.

The supplied `sdk.Backups` adapter provides read-only inventory. Its `Delete` method returns `ErrOwnershipUnavailable` without issuing an HTTP request. Without an independent ownership verifier, `BackupManager` lists records, skips them and reports the limitation. Enabling backup evaluation or supplying a local registry does not enable deletion.

## Custom backup integrations

Custom Go integrations can supply a `BackupAPI` and `OwnershipVerifier`. They must independently establish ownership and safe deletion eligibility, including protection against changes between verification and deletion. A local registry or record rechecks alone do not provide those guarantees. Retention decisions and partial failures are reported separately from snapshot publication.

The supplied SDK adapter remains read-only because the reviewed API contract does not establish the required ownership and conditional deletion guarantees. Investigation is tracked in [backup ownership and deletion issue #2](https://github.com/easqr/iscconfigexporter/issues/2).

## Object mappings

The optional mapping action uses V2026 `GetObjectMappings(ctx, sourceOrg)`, GET `/v2026/configuration-hub/object-mappings/{sourceOrg}`. The method returns one array and exposes no pagination controls. It is independent of configuration exports and backup evaluation. See [mapping retrieval](MAPPINGS.md) for permissions, bundle format and usage.

## References

- [Pinned Go SDK source](https://github.com/sailpoint-oss/golang-sdk/tree/v2.7.106)
- [SailPoint SaaS configuration workflow](https://developer.sailpoint.com/docs/extensibility/configuration-management/saas-configuration/)
- [Export job status model](https://developer.sailpoint.com/docs/tools/sdk/powershell/beta/models/sp-config-export-job-status/)
- [Configuration Hub backup listing](https://developer.sailpoint.com/docs/api/v2026/list-backups/)
- [Configuration Hub backup deletion](https://developer.sailpoint.com/docs/api/v2026/delete-backup/)

The pinned SDK source defines the calls used by this version. Current online documentation may describe other SDK versions or route families.
