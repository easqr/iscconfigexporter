# Dependency and license review

The CLI uses SailPoint Go SDK **v2.7.106**. The table below lists external modules imported by the CLI, checked against the current dependency graph. Full license texts and notices are retained in [licenses](licenses/).

The exporter source has no assigned open-source license. Dependency licenses do not assign a license to the exporter itself. The owner decision is tracked in [source-license issue #6](https://gitlab.com/easqrtech/iam/iscconfigexporter/-/work_items/6).

| Runtime module | Version | License |
|---|---|---|
| sailpoint-oss/golang-sdk/v2 | v2.7.106 | MIT |
| hashicorp/go-retryablehttp | v0.7.8 | MPL-2.0 |
| hashicorp/go-cleanhttp | v0.5.2 | MPL-2.0 |
| golang.org/x/oauth2 | v0.30.0 | BSD-3-Clause |
| gopkg.in/validator.v2 | v2.0.1 | Apache-2.0 |
| fsnotify/fsnotify | v1.9.0 | BSD-3-Clause |
| go-viper/mapstructure/v2 | v2.4.0 | MIT |
| pelletier/go-toml/v2 | v2.2.4 | MIT |
| sagikazarmark/locafero | v0.10.0 | MIT |
| sourcegraph/conc | v0.3.1-0.20240121214520-5f936abd7ae8 | MIT |
| spf13/afero | v1.14.0 | Apache-2.0 |
| spf13/cast | v1.9.2 | MIT |
| spf13/pflag | v1.0.7 | BSD-3-Clause |
| spf13/viper | v1.20.1 | MIT |
| subosito/gotenv | v1.6.0 | MIT |
| golang.org/x/sys | v0.35.0 | BSD-3-Clause |
| golang.org/x/text | v0.28.0 | BSD-3-Clause |
| gopkg.in/yaml.v3 | v3.0.1 | MIT and Apache-2.0 (see its license/notice) |

The SDK uses retryablehttp and validator. The HashiCorp HTTP dependencies are licensed under MPL-2.0, which has file-level copyleft obligations. No dependency source is modified. Any later distribution must preserve the license notices and satisfy the applicable source-availability obligations for the MPL-covered files (upstream source: [go-retryablehttp](https://github.com/hashicorp/go-retryablehttp/tree/v0.7.8) and [go-cleanhttp](https://github.com/hashicorp/go-cleanhttp/tree/v0.5.2)).

Additional dependency-test notices are retained for: kr/pretty v0.3.1 (MIT), kr/text v0.2.0 (MIT), rogpeppe/go-internal v1.9.0 (BSD-3-Clause), gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c (BSD-style two-clause). They are not linked into the CLI. License notices are retained for traceability. Other modules can appear in the SDK module graph without being linked into the CLI. Contributor instructions for refreshing the inventory are in [development](DEVELOPMENT.md#dependency-review).
