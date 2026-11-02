# ISC Configuration Exporter

Reusable Go library and thin CLI for SailPoint ISC configuration export, deterministic object organization, safe snapshot publication and optional backup lifecycle evaluation. No Git, import, drift, scheduling, HTTP service or deployment functionality.

Requires Go 1.27 or later. Build with `go build ./cmd/isc-exporter`; run checks with `make check`. Tests use doubles, never a live tenant.

Copy `config.example.json` to `config.json`. `go run ./cmd/isc-exporter -help` and `go run ./cmd/isc-exporter -validate -config config.json` require no credentials.

Implementation proceeds through six locally committed slices. See docs for architecture and operational details as they are added. No project license grant is implied; dependency licenses remain applicable.
