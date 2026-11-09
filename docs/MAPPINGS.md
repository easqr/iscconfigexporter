# Optional Configuration Hub mapping retrieval

Mapping retrieval saves Configuration Hub Object Mappings to a separate local archive. It reads tenant data without changing it, and does not run a configuration export or backup evaluation. Ordinary configuration exports do not require mapping permissions.

## CLI

Use the existing exporter policy and native SDK connection settings for the **target/current** tenant:

```sh
isc-exporter -config exporter-sandbox.json -retrieve-mappings \
  -mapping-source-org invented-prod \
  -mapping-output ./mapping-archives/prod-to-sandbox-001
```

The policy's `tenant` declares the target tenant (for example `invented-sb`). Native SDK `config.json` or `SAIL_*` connection settings select the authenticated API origin. The exporter records both separately; it does not attest that the configured origin belongs to the declared tenant. Confirm those settings describe the intended target. No credentials are copied into provenance.

`-mapping-source-org` is required. The SDK fetches mappings between that source organization and the authenticated current organization. Use the literal `default` for mappings not associated with a particular source organization. `default` is a group selector, **not** an asserted source tenant. Offline consumers must explicitly decide how to bind that group to a real comparison context.

`-mapping-output` is required and must be a **new directory**, outside the configured configuration export tree. The archive cannot be inside the export directory or contain that directory, including through symbolic links. These checks run before credentials are loaded. A later retrieval requires another new directory; existing bundles are never refreshed in place. Mapping-only options without `-retrieve-mappings`, and retrieval combined with `-cleanup-preview`, are rejected. Even if backup evaluation is enabled in the exporter policy, retrieval does not run it.

Optional `-mapping-export-job EXISTING-JOB-ID` records operator-supplied correlation with an earlier export. The endpoint is synchronous and returns no export/job ID of its own. The correlation is not verified remotely, and no export job is started to obtain it.

Add `-validate` to validate policy, mapping options, and destination separation without loading SDK credentials, issuing requests, or creating directories. Local validation does not check remote permissions. Retrieval uses the policy's existing timeout/retry/backoff settings. See [authentication limits](CONFIGURATION.md#authentication-limits) before relying on timeout or cancellation during SDK token acquisition.

Successful retrieval writes one JSON publication record to standard output containing directory and provenance, **not mapping values**. Permission, transport, malformed-response, validation, and publication errors return a nonzero exit status. HTTP error bodies are withheld.

## Archive files (schema 1)

```text
prod-to-sandbox-001/
  mappings.json
  provenance.json
```

`mappings.json` is the original response array, byte for byte. No substitutions, sorting, normalization, status filtering, or JSONPath interpretation occur during retrieval. Enabled and disabled records, original JSONPath, source/target strings, mapping IDs, timestamps, unknown fields, array order, and numeric precision in unknown fields survive.

The response must be a UTF-8 JSON array of mapping objects. Empty `[]` is valid. Duplicate JSON keys, duplicate mapping IDs and content after the array are rejected.

| Mapping field | JSON type | Required? | Requirement |
| --- | --- | --- | --- |
| `objectMappingId` | String | Yes | Nonempty and unique within the response |
| `objectType` | String | Yes | Nonempty |
| `jsonPath` | String | Yes | Nonempty; retrieval does not interpret the expression |
| `sourceValue` | String | Yes | May be empty |
| `targetValue` | String | Yes | May be empty |
| `enabled` | Boolean | Yes | Both enabled and disabled records are preserved |
| `created`, `modified` | String | No | Must be strings when present |
| Other fields | Any valid JSON | No | Preserved without changing their values |

The pinned SDK also checks its response schema and can reject a response before these storage checks run. A successful response is saved with its original bytes.

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

Provenance records where and when the archive was retrieved; its checksum lets you detect changed bytes. It does not authenticate the source or prove the archive and an export were captured together. Mappings and an independently produced snapshot may represent different times. The mapping bundle is not an exporter object snapshot and has no snapshot `manifest.json`.

### Storage and recovery

The exporter validates the archive, writes it to a temporary directory, syncs it to storage and renames it into the requested destination. The destination must not already exist. Files use permissions 0600 and directories 0700. Use private local storage with directory sync and atomic rename support; keep saved mapping values outside source control.

A lock prevents cooperating retrievals from writing the same archive at once. Failures before the final rename remove temporary staging after ordinary error returns; a crash can leave a temporary directory or lock. Check that no retrieval is active before removing a stale lock. A directory-sync error after the final rename means the complete archive is visible, but its survival after a crash is uncertain. Verify the files and checksum before deciding whether another retrieval is needed.

Archives are not overwritten, refreshed in place or pruned automatically. Use a new directory for each retrieval.

## Library

The following function uses an existing exporter policy and an SDK adapter (or another implementation of `MappingAPI`). It returns the saved archive location and provenance, or an error:

```go
package example

import (
    "context"

    exporter "github.com/easqr/iscconfigexporter"
)

func saveMappings(ctx context.Context, c exporter.Config, api exporter.MappingAPI) (exporter.MappingPublication, error) {
    timeout, err := exporter.ParseDuration(c.Timeout)
    if err != nil {
        return exporter.MappingPublication{}, err
    }
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()
    saved, err := exporter.RetrieveMappings(ctx, api, exporter.MappingRequest{
        SourceOrg: "invented-prod", TargetTenant: c.Tenant,
    })
    if err != nil {
        return exporter.MappingPublication{}, err
    }
    return (exporter.MappingFileStore{
        Directory: "./mapping-archives/prod-to-sandbox-001", ExportRoot: c.Output,
    }).Publish(ctx, saved)
}
```

`MappingAPI` is a separate interface for optional mapping retrieval. `RetrieveMappings` performs no filesystem operations. `MappingFileStore.Publish` revalidates the checksum/count/provenance and copies input bytes before writing. `ExportRoot` enforces output separation when supplied; the CLI always supplies it. Direct library callers must identify the export root or independently choose separate storage. The root library and `snapshot` package remain SDK-free; generated models stay inside `sdk`.

## Offline differ handoff

Configdiffer is a separate, currently unpublished project. Its source checkout contains the *Export input contract* and *Comparison policy v2* documents. Once you have built its CLI, you can import tenant-specific mappings as follows:

```sh
isc-configdiffer --mode drift \
  --left /data/production --left-mode snapshot --left-env production \
  --right /data/sandbox --right-mode snapshot --right-env sandbox \
  --policy /policies/portable-policy.json \
  --hub ./mapping-archives/prod-to-sandbox-001/mappings.json \
  --hub-source-tenant invented-prod --hub-target-tenant invented-sb
```

Use a policy with `version:2`, `id`, and `profile:"isc-portable-v2"` at the policy path above. The saved bytes' SHA-256 can be checked against `provenance.json`. When given the bare array path above, the differ uses explicit source/target context and hashes the file. Passing the bundle directory instead validates `provenance.json` automatically; the sidecar records retrieval context and integrity rather than remote authenticity. Its strict importer can reject additional response fields or unsupported active JSONPath expressions, even when retrieval validly preserved them. Retrieval success does not imply a rule is supported or applicable in a comparison. Disabled unsupported paths stay inert; unsupported active rules must never be silently dropped. Hub substitutions remain separate from strict directional environment assertions.

## API and permissions

This version uses the pinned Go SDK v2.7.106 V2026 endpoint, GET `/v2026/configuration-hub/object-mappings/{sourceOrg}`, with no pagination controls.

The required permission is `sp:config-object-mapping:read`. Ordinary exports do not require it. See [SailPoint's Configuration Hub SDK reference](https://developer.sailpoint.com/docs/tools/sdk/typescript/configuration_hub/methods/configuration-hub/) for permission and source-group semantics, and [Mapping Objects](https://documentation.sailpoint.com/saas/help/confighub/config_hub_mapping.html) for mapping fields and enabled behavior. Online SDK documentation can use a different route family from this pinned Go integration.

Retrieval preserves mappings; it does not apply substitutions or claim parity with Configuration Hub comparison. For implementation and automated verification details, see the [SDK integration review](development/SDK_INTEGRATION.md).
