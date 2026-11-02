# Dependency and license review

Pinned SDK version: github.com/sailpoint-oss/golang-sdk/v2 v2.7.106 (Go proxy latest stable checked 2026-11-01). Generated beta SP-Config and v2026 Configuration Hub clients are imported directly, avoiding the root SDK's Viper configuration stack. `go mod tidy` retains the SDK module graph, but the shipped CLI links the runtime modules below. Reviewed local module license files; complete notices are retained in docs/licenses.

| Runtime module | Version | License |
|---|---|---|
| sailpoint-oss/golang-sdk/v2 | v2.7.106 | MIT |
| hashicorp/go-retryablehttp | v0.7.8 | MPL-2.0 |
| hashicorp/go-cleanhttp | v0.5.2 | MPL-2.0 |
| golang.org/x/oauth2 | v0.30.0 | BSD-3-Clause |
| gopkg.in/validator.v2 | v2.0.1 | Apache-2.0 |

The SDK requires retryablehttp and validator. MPL is file-level copyleft: do not describe these dependencies as permissively licensed or strip their notices. No dependency source was modified. Any later distribution must preserve the license notices and satisfy the applicable source-availability obligations for the MPL-covered files (upstream source: https://github.com/hashicorp/go-retryablehttp/tree/v0.7.8 and https://github.com/hashicorp/go-cleanhttp/tree/v0.5.2). This implementation has not published or distributed a release artifact or assigned an open-source license to the component itself.

Additional dependency-test modules downloaded during tidy: kr/pretty v0.3.1 (MIT), kr/text v0.2.0 (MIT), rogpeppe/go-internal v1.9.0 (BSD-3-Clause), gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c (BSD-style two-clause). They are not linked into the CLI. License notices are retained for traceability. Other modules may remain in the SDK's module graph without being imported or included in our runtime; rerun the inventory when imports or versions change.

The functional reference colab-isc-config-hub-history is MIT licensed, as stated by its repository. Its architecture was reviewed; no implementation source was copied, and none of its tokenization, restore or Git tooling was reused. Tests use only Go's standard testing package. Static validation is go vet; no unexecuted vulnerability scanner is claimed.
