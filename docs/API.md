# Verified API findings

Reviewed 2026-11-01 against SailPoint Go SDK v2.7.106 (latest stable v2 release; v3 is a separate major version), generated beta API source and official documentation:

- https://github.com/sailpoint-oss/golang-sdk/tree/v2.7.106
- https://developer.sailpoint.com/docs/extensibility/configuration-management/saas-configuration/
- https://github.com/sailpoint-oss/colab-isc-config-hub-history/blob/main/scripts/backup.mjs

The reference discovers exportable types, initiates one asynchronous SP-Config job, polls and downloads an objects envelope. We reproduce that flow, without its noise stripping, array sorting, Git integration or directory-wide deletion. No reference implementation source was copied.

SDK beta methods verified in source: ListSpConfigObjects, ExportSpConfig with ExportPayload.IncludeTypes, GetSpConfigExportStatus, GetSpConfigExport. Discovery is an unpaginated array; these SDK methods expose no limit/offset. Download is one JSON document, not paginated. No hard object limit is documented here; memory usage is proportional to the full export. Job results have expiration metadata: download promptly. Only COMPLETE allows download. Failed, cancelled, unexpected states fail closed.

The generated ConfigObject omits unknown envelope fields including signatures. The SDK restores its raw HTTP response body after decoding; the adapter reads that body to preserve all original fields and numeric precision. No separate REST export implementation is required.

Configuration/authentication and API construction use the root SDK's NewDefaultConfiguration/NewConfiguration and NewAPIClient. Credentials, token exchange and caching belong to the SDK. SP-Config and Hub share one root client, with a context-aware gate protecting the SDK's mutable caches. The SDK HTTP client's retry engine handles transient GET responses; POST initiation is never retried. No custom token source, OAuth endpoint construction or engine retry loop remains. SDK discovery panic text and API error bodies are withheld. See CONFIGURATION.md for verified native token timeout/refresh limitations and SDK_REUSE_AUDIT.md for the reuse review. No live tenant was contacted.

## Configuration Hub ownership limitation

Official Configuration Hub endpoints and pinned SDK v2026 source were reviewed:

- https://developer.sailpoint.com/docs/api/v2026/list-backups/
- https://developer.sailpoint.com/docs/api/v2026/delete-backup/
- SDK api_v2026/api_configuration_hub.go and model_backup_response_1.go.

ListBackups returns an unpaginated array (only status filtering is exposed), with jobId, tenant, type, requesterName, created/completed, userCanDelete, isPartial and backupType. DeleteBackup accepts an ID; permissions may still change and return 403, records may disappear (404). COMPLETE plus isPartial=false indicates a fully successful record; userCanDelete must be explicit true for eligibility. Uploaded configurations have distinct endpoints and are not interchangeable with ordinary backups. No independent ownership marker or conditional deletion token is present. requesterName/name/type are not ownership evidence.

Official SaaS configuration documentation describes backups and exports as related operations using the same APIs, but the reviewed contract does not guarantee that a specific SP-Config export job ID is the persistent Configuration Hub backup ID, or that every SP-Config export creates a deletable Configuration Hub record. Shared terminology and similar response fields cannot establish that association. No extra backup creation or upload is performed. We cannot safely populate a Configuration Hub ownership registry from the SP-Config response alone.

Therefore the shipped SDK Backups adapter is an implemented read-only inventory: Delete always returns ErrOwnershipUnavailable without HTTP traffic. It does not auto-register records. BackupManager without an independent verifier lists records, skips all and reports the limitation. Enabling cleanup never bypasses this. FileRegistry is implemented for future proven associations; a registry record is trusted local provenance, not independent remote ownership proof. Protect its location from writes by other users. No production verifier is shipped.

The generic BackupManager implements both retention policies, preview, duplicate detection, newest-success protection (all ties), tenant/creation matching, current-operation protection, state and ownership rechecks, successor availability and partial deletion reports. Its destructive path is tested exclusively with fake independently verified providers. A future platform must supply a genuinely verified BackupAPI and OwnershipVerifier before using that path. Rechecks do not eliminate a remote time-of-check/time-of-use race; the reviewed API has no conditional delete guarantee. The built-in adapter consequently remains non-destructive. A future destructive adapter must resolve that limitation as well as ID association, not simply return true from a verifier.

## Optional read-only object mappings

The separately invoked mapping operation uses the pinned V2026 `GetObjectMappings(ctx, sourceOrg)` SDK method, GET `/v2026/configuration-hub/object-mappings/{sourceOrg}`, with `sp:config-object-mapping:read`. The source organization is distinct from the authenticated target/current organization; `default` selects unassociated mappings. This SDK method returns one array and exposes no pagination controls. Original response bytes are preserved after the generated decoder succeeds; no custom REST/authentication implementation is added. Generated Beta/V2026 clients retain native per-version token caches; mapping and backup reads share the existing V2026 client. See [MAPPINGS.md](MAPPINGS.md) for the schema, provenance and synthetic verification. Normal exports do not invoke this optional endpoint.
