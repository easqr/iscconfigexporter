# Dependency and license review

Pinned SDK version: github.com/sailpoint-oss/golang-sdk/v2 v2.7.106, the latest stable v2 release. The newer v3 major was inspected at v3.1.74 during the native-authentication review; its token-helper limitations are the same, and migration would also change API endpoints, so this change keeps the verified v2 API contract. Native configuration now imports the root SDK and its Viper stack. Runtime modules below were identified with `go list -deps` for ./cmd/isc-exporter. Reviewed local module license files; complete notices are retained in docs/licenses. Viper is used directly through a private instance to load local SDK-format config.json without the SDK's deprecated home-directory fallback or global parser state.

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

The SDK requires retryablehttp and validator. MPL is file-level copyleft: do not describe these dependencies as permissively licensed or strip their notices. No dependency source was modified. Any later distribution must preserve the license notices and satisfy the applicable source-availability obligations for the MPL-covered files (upstream source: https://github.com/hashicorp/go-retryablehttp/tree/v0.7.8 and https://github.com/hashicorp/go-cleanhttp/tree/v0.5.2). This implementation has not published or distributed a release artifact or assigned an open-source license to the component itself.

Additional dependency-test modules downloaded during tidy: kr/pretty v0.3.1 (MIT), kr/text v0.2.0 (MIT), rogpeppe/go-internal v1.9.0 (BSD-3-Clause), gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c (BSD-style two-clause). They are not linked into the CLI. License notices are retained for traceability. Other modules may remain in the SDK's module graph without being imported or included in our runtime; rerun the inventory when imports or versions change.

The functional reference colab-isc-config-hub-history is MIT licensed, as stated by its repository. Its architecture was reviewed; no implementation source was copied, and none of its tokenization, restore or Git tooling was reused. Tests use only Go's standard testing package. Static validation is go vet; no unexecuted vulnerability scanner is claimed.
