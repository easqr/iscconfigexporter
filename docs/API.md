# Verified API findings

Reviewed 2026-11-01 against SailPoint Go SDK v2.7.106 (latest stable module returned by the Go proxy), generated beta API source and official documentation:

- https://github.com/sailpoint-oss/golang-sdk/tree/v2.7.106
- https://developer.sailpoint.com/docs/extensibility/configuration-management/saas-configuration/
- https://github.com/sailpoint-oss/colab-isc-config-hub-history/blob/main/scripts/backup.mjs

The reference discovers exportable types, initiates one asynchronous SP-Config job, polls and downloads an objects envelope. We reproduce that flow, without its noise stripping, array sorting, Git integration or directory-wide deletion. No reference implementation source was copied.

SDK beta methods verified in source: ListSpConfigObjects, ExportSpConfig with ExportPayload.IncludeTypes, GetSpConfigExportStatus, GetSpConfigExport. Discovery is an unpaginated array; these SDK methods expose no limit/offset. Download is one JSON document, not paginated. No hard object limit is documented here; memory usage is proportional to the full export. Job results have expiration metadata: download promptly. Only COMPLETE allows download. Failed, cancelled, unexpected states fail closed.

The generated ConfigObject omits unknown envelope fields including signatures. The SDK restores its raw HTTP response body after decoding; the adapter reads that body to preserve all original fields and numeric precision. No separate REST export implementation is required.

Authentication uses the SDK's supported ContextOAuth2 TokenSource, backed by OAuth client credentials. This avoids the generated client's mutable cached token and context-free token acquisition. SDK automatic retries and HTTP logging are disabled. Engine retries bounded transient read errors only (429, 408, 5xx). POST initiation is never retried: an ambiguous failure may leave a remote job and requires operator investigation. API error bodies are withheld because they may contain sensitive data. No live tenant was contacted.
