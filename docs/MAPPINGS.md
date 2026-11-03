# Optional Configuration Hub mapping retrieval

Mapping retrieval is a separate read-only operation. It reuses the existing SDK configuration/authentication and the existing V2026 Configuration Hub client. It does not run SP-Config discovery, start an export or backup, poll a job, run cleanup, or mutate tenant data. Ordinary configuration exports never call mapping endpoints and require no additional mapping permission.

## CLI

Use the existing exporter policy and native SDK connection settings for the **target/current** tenant:

```sh
isc-exporter -config exporter-sandbox.json -retrieve-mappings \
  -mapping-source-org invented-prod \
  -mapping-output ./mapping-archives/prod-to-sandbox-001
```

The policy's `tenant` declares the target tenant (for example `invented-sb`). Native SDK `config.json` or `SAIL_*` connection settings select the authenticated API origin. The exporter records both separately; it does not attest that the configured origin belongs to the declared tenant. Confirm those settings describe the intended target. No credentials are copied into provenance.

`-mapping-source-org` is required. The SDK fetches mappings between that source organization and the authenticated current organization. Use the literal `default` for mappings not associated with a particular source organization. `default` is a group selector, **not** an asserted source tenant. Offline consumers must explicitly decide how to bind that group to a real comparison context.

`-mapping-output` is required and must be a **new directory**, outside the configured configuration export tree. Ancestor and descendant overlap, including overlap through existing symlinks, is rejected before SDK loading. A later retrieval requires another new directory; existing bundles are never refreshed in place. Mapping-only options without `-retrieve-mappings`, and retrieval combined with `-cleanup-preview`, are rejected. Even if backup cleanup is enabled in the exporter policy, retrieval never invokes it.

Optional `-mapping-export-job EXISTING-JOB-ID` records operator-supplied correlation with an earlier export. The endpoint is synchronous and returns no export/job ID of its own. The correlation is not verified remotely, and no export job is started to obtain it.

Add `-validate` to validate policy, mapping options, and destination separation without loading SDK credentials, issuing requests, or creating directories. Local validation does not check remote permissions. Retrieval uses the policy's existing timeout/retry/backoff settings. Native SDK token acquisition still has the limitations documented in [CONFIGURATION.md](CONFIGURATION.md); generated Beta and V2026 clients cache tokens separately, while mappings and backups share the V2026 cache and adapter gate.

Successful retrieval writes one JSON publication record to standard output containing directory and provenance, **not mapping values**. Permission, transport, malformed-response, validation, and publication errors return the existing CLI nonzero status. HTTP error bodies are withheld. No live tenant calls were used to verify this feature.

## Bundle contract, schema 1

```text
prod-to-sandbox-001/
  mappings.json
  provenance.json
```

`mappings.json` is the original response array, byte for byte. No substitutions, sorting, normalization, status filtering, or JSONPath interpretation occur during retrieval. Enabled and disabled records, original JSONPath, source/target strings, mapping IDs, timestamps, unknown fields, array order, and numeric precision in unknown fields survive.

Storage validation rejects duplicate JSON keys, invalid UTF-8/trailing JSON, non-array responses, non-object records, duplicate mapping IDs, missing/non-string `objectMappingId`, `objectType`, `jsonPath`, `sourceValue`, or `targetValue`, missing/non-boolean `enabled`, and non-string supplied `created`/`modified`. Identity/type/path strings must be nonempty; source and target values can be empty strings. Empty `[]` is a valid retrieval. The pinned SDK may also reject responses that fail its generated schema decoder; raw bytes are preserved from its restored response body after successful decoding.

`provenance.json` contains:

| Field | Meaning |
| --- | --- |
| `schema` | Bundle/provenance version 1 |
| `retrieval_id` | Random local retrieval identifier, independent of remote jobs |
| `source_org` | Exact requested source organization/group |
| `target_tenant` | Operator-declared exporter policy tenant |
| `api_origin` | Actual SDK request origin, without credentials or query parameters |
| `api_version` | `v2026` for the pinned SDK integration |
| `endpoint` | `/v2026/configuration-hub/object-mappings/{sourceOrg}` |
| `retrieved` | UTC retrieval-completion timestamp |
| `export_job_id` | Optional operator-supplied correlation, not verified association |
| `count` | Total records, including disabled mappings |
| `sha256` | SHA-256 of exact `mappings.json` bytes |

These are retrieval provenance and byte integrity, not authenticity or a transaction with a configuration export. Mappings and an independently produced snapshot may represent different times. The mapping bundle is not an exporter object snapshot and has no snapshot `manifest.json`.

Publication validates the bytes/provenance before writing. A private staging directory is written and synced, then renamed into place as a complete bundle. Files use 0600 and directories 0700. An output lock serializes cooperating publishers for the same directory. Parent storage must be trusted/private and support local POSIX directory sync and atomic rename, as with configuration snapshot publication. It must not be changed by unrelated writers during publication. Pre-publication failures preserve existing outputs and clean up staging; a crash can leave a staging directory or lock that requires inspection. A failure syncing the parent after rename reports that the bundle is visible but durability is uncertain. No automatic pruning or destructive replacement is provided. Saved mappings can contain sensitive configuration values; retain them outside source control.

## Library

```go
// c is the ordinary exporter policy; api uses sdk.New or sdk.NewWithConfiguration.
ctx, cancel := context.WithTimeout(ctx, timeout)
defer cancel()
saved, err := exporter.RetrieveMappings(ctx, api, exporter.MappingRequest{
    SourceOrg: "invented-prod", TargetTenant: c.Tenant,
})
// Handle err before publishing.
publication, err := (exporter.MappingFileStore{
    Directory: "./mapping-archives/prod-to-sandbox-001", ExportRoot: c.Output,
}).Publish(ctx, saved)
```

`MappingAPI` is a separate interface; existing `API` implementations and external exporter consumers remain compatible. `RetrieveMappings` performs no filesystem operations. `MappingFileStore.Publish` revalidates the checksum/count/provenance and copies input bytes before writing. `ExportRoot` enforces output separation when supplied; the CLI always supplies it. Direct library callers must identify the export root or independently choose separate storage. The root library and `snapshot` package remain SDK-free; generated models stay inside `sdk`.

## Offline differ handoff

For tenant-specific source mappings, import the saved array using the existing differ action:

```sh
isc-configdiffer --left /data/production --left-mode snapshot \
  --right /data/sandbox --right-mode snapshot \
  --hub ./mapping-archives/prod-to-sandbox-001/mappings.json \
  --hub-source-tenant invented-prod --hub-target-tenant invented-sb
```

The saved bytes' SHA-256 can be checked against `provenance.json`. The differ currently takes explicit source/target context and hashes the mapping file; it does not automatically read or attest the sidecar. Its strict importer can reject additional response fields or unsupported active JSONPath expressions, even when retrieval validly preserved them. Retrieval success does not imply a rule is supported or applicable in a comparison. Disabled unsupported paths stay inert; unsupported active rules must never be silently dropped. Hub substitutions remain separate from strict directional environment assertions.

## API evidence and verification

The implementation uses the locally inspected pinned Go SDK **v2.7.106**, `api_v2026.ConfigurationHubAPIService.GetObjectMappings(ctx, sourceOrg).Execute()`, which returns `[]ObjectMappingResponse`, HTTP response, and error. The generated request is GET `/v2026/configuration-hub/object-mappings/{sourceOrg}`. This SDK contract exposes no pagination controls. There is no second REST implementation, API-version fallback, or dependency upgrade.

The required permission is `sp:config-object-mapping:read`, documented in [SailPoint's Configuration Hub SDK reference](https://developer.sailpoint.com/docs/tools/sdk/typescript/configuration_hub/methods/configuration-hub/). That current SDK reference uses a different route family; this integration deliberately uses the verified pinned Go V2026 contract. Source organization and `default` semantics are also documented there. JSON mapping fields and enabled behavior are described in [Mapping Objects](https://documentation.sailpoint.com/saas/help/confighub/config_hub_mapping.html). No built-in substitutions or parity with proprietary Configuration Hub comparison are assumed.

Synthetic tests cover the exact SDK endpoint, GET-only behavior, native authentication and per-version caches, retries/permanent errors, cancellation, raw-byte and unknown-field preservation, disabled status, default scope, strict response validation, provenance, file permissions, no replacement, export-tree separation including symlink aliases, error redaction, CLI operation isolation, and local validation with no SDK loading or writes. A separate synthetic differ test exercises retrieval → bundle publication → offline import. Tests use no tenant credentials, real exports, or network services.
