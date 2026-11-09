GO ?= go
GO_TOOLCHAIN ?= go1.27.3
GOVULNCHECK_VERSION := v1.9.0
TEST_TIMEOUT ?= 5m
VERSION ?=
BUILD_OUTPUT ?= isc-exporter
BUILD_LDFLAGS = -X gitlab.com/easqrtech/iam/iscconfigexporter/internal/buildinfo.releaseVersion=$(VERSION)

export GOTOOLCHAIN := $(GO_TOOLCHAIN)
export GOFLAGS := $(GOFLAGS) -mod=readonly

.PHONY: check release-check release-build toolchain fmt fmt-check vet test test-race vuln smoke build

# Keep the default target suitable for local development and future CI jobs.
check: toolchain fmt-check vet test test-race smoke

release-check: check vuln

toolchain:
	$(GO) version

fmt:
	@formatter="$$($(GO) env GOROOT)/bin/gofmt"; \
	git ls-files -z '*.go' | xargs -0 "$$formatter" -w

fmt-check:
	@formatter="$$($(GO) env GOROOT)/bin/gofmt"; \
	files="$$(git ls-files -z '*.go' | xargs -0 "$$formatter" -l)" || exit $$?; \
	if [ -n "$$files" ]; then \
		printf 'Go files need formatting (run make fmt):\n%s\n' "$$files"; \
		exit 1; \
	fi

vet:
	$(GO) vet ./...

test:
	$(GO) test -timeout=$(TEST_TIMEOUT) ./...

test-race:
	$(GO) test -race -timeout=$(TEST_TIMEOUT) ./...

vuln: toolchain
	@printf 'govulncheck %s\n' '$(GOVULNCHECK_VERSION)'
	$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) -show verbose ./...

smoke:
	@set -eu; \
	tmp="$$(mktemp -d "$${TMPDIR:-/tmp}/isc-exporter-smoke.XXXXXX")"; \
	trap 'rm -rf "$$tmp"' EXIT; \
	trap 'exit 1' HUP INT TERM; \
	$(GO) build -o "$$tmp/isc-exporter" ./cmd/isc-exporter; \
	"$$tmp/isc-exporter" -help >"$$tmp/help.txt" 2>&1; \
	"$$tmp/isc-exporter" -version; \
	"$$tmp/isc-exporter" -config exporter.example.json -validate

build:
	$(GO) build -ldflags '$(BUILD_LDFLAGS)' -o '$(BUILD_OUTPUT)' ./cmd/isc-exporter

# Release identification must match a tag at a clean checkout, without moving tags.
release-build:
	@test -n '$(VERSION)' || { echo 'Set VERSION to the release tag, for example v0.7.0'; exit 1; }
	@test -z "$$(git status --porcelain)" || { echo 'Release builds require a clean checkout'; exit 1; }
	@test "$$(git rev-parse 'refs/tags/$(VERSION)^{commit}')" = "$$(git rev-parse HEAD)" || { echo 'VERSION must name a release tag at HEAD'; exit 1; }
	$(MAKE) release-check
	$(MAKE) build VERSION='$(VERSION)' BUILD_OUTPUT='$(BUILD_OUTPUT)'
