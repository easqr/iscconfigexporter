# Component boundaries

The root package owns Config, Engine, API, Store, Lifecycle, Registry, BackupAPI, Manifest and Snapshot. SDK models remain exclusively in `sdk`. The CLI composes the library. No service or platform persistence assumptions are imposed. API doubles and HTTP RoundTripper doubles exercise the actual adapter without live traffic.

Flow: discover exportable types → validate selection → initiate once → poll COMPLETE → download → validate every envelope → organize → publish → optional cleanup. A successful export is distinct from cleanup success.

Organization is pure. It requires matching tenant, selected types, nonempty stable identity, positive integer version and an object payload. Unknown fields and signatures survive. JSON keys are sorted using encoding/json; arrays keep source order and numbers use json.Number to avoid float precision loss. Duplicate JSON keys are rejected. Operational timestamps are only in the manifest. This is deterministic JSON, not a cryptographic JSON canonicalization standard; no signatures are regenerated or claimed verified.

IDs become `id-<UTF-8 hex>.json`, a reversible portable encoding that avoids colon substitution and case-fold collisions. Slash, backslash, control characters, dot identities and IDs over 100 bytes fail validation. Tenant/type names use restricted portable identifiers; device names are forbidden. Case-folded path collisions fail closed.

Consumers use Manifest.Artifacts (relative paths, SHA-256), Counts, Types, JobID, Completed and Changes. Artifacts contain the complete original object envelope; top-level download metadata remains operational input, not individual object content. Fetch and Organize may be used independently of filesystem publication.
