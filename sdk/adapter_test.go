package sdk

import (
	"context"
	"errors"
	exporter "github.com/elianderson/iscutilities/configexporter"
	sailpoint "github.com/sailpoint-oss/golang-sdk/v2"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestSDKRequests(t *testing.T) {
	c := exporter.DefaultConfig()
	c.Tenant = "example"
	posts := 0
	a, e := newTestAdapter(t, c, roundTrip(func(r *http.Request) (*http.Response, error) {
		body := ""
		code := 200
		switch r.URL.Path {
		case "/oauth/token":
			body = `{"access_token":"token","token_type":"Bearer","expires_in":3600}`
		case "/beta/sp-config/config-objects":
			body = `[{"objectType":"ROLE","exportable":true},{"objectType":"IGNORED","exportable":false}]`
		case "/beta/sp-config/export":
			posts++
			code = 202
			body = `{"jobId":"j","status":"NOT_STARTED","type":"EXPORT","modified":"2026-01-01T00:00:00Z","created":"2026-01-01T00:00:00Z"}`
		case "/beta/sp-config/export/j":
			body = `{"jobId":"j","status":"COMPLETE","type":"EXPORT","created":"2026-01-01T00:00:00Z","modified":"2026-01-01T00:00:00Z"}`
		case "/beta/sp-config/export/j/download":
			body = `{"objects":[],"unknown":{"preserved":true}}`
		default:
			t.Fatalf("unexpected request %s", r.URL)
		}
		return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}))
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	v, e := a.Discover(ctx)
	if e != nil || len(v) != 1 {
		t.Fatal(v, e)
	}
	id, e := a.Start(ctx, v)
	if e != nil || id != "j" || posts != 1 {
		t.Fatal(id, e)
	}
	s, e := a.Status(ctx, id)
	if e != nil || s != "COMPLETE" {
		t.Fatal(s, e)
	}
	raw, e := a.Download(ctx, id)
	if e != nil || !strings.Contains(string(raw), "unknown") {
		t.Fatal(string(raw), e)
	}
}

func TestSDKEndToEnd(t *testing.T) {
	raw, e := os.ReadFile("../testdata/export.json")
	if e != nil {
		t.Fatal(e)
	}
	c := exporter.DefaultConfig()
	c.Tenant = "example"
	c.Output = t.TempDir()
	c.Include = []string{"ROLE", "SOURCE"}
	c.Backup = exporter.BackupConfig{Enabled: true, Policy: "keep_latest"}
	requests := []string{}
	a, e := newTestAdapter(t, c, roundTrip(func(r *http.Request) (*http.Response, error) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		body := ""
		code := 200
		switch r.URL.Path {
		case "/oauth/token":
			body = `{"access_token":"token","token_type":"Bearer","expires_in":3600}`
		case "/beta/sp-config/config-objects":
			body = `[{"objectType":"ROLE","exportable":true},{"objectType":"SOURCE","exportable":true}]`
		case "/beta/sp-config/export":
			code = 202
			body = `{"jobId":"j","status":"NOT_STARTED","type":"EXPORT","created":"2026-01-01T00:00:00Z","modified":"2026-01-01T00:00:00Z"}`
		case "/beta/sp-config/export/j":
			body = `{"jobId":"j","status":"COMPLETE","type":"EXPORT","created":"2026-01-01T00:00:00Z","modified":"2026-01-01T00:00:00Z"}`
		case "/beta/sp-config/export/j/download":
			body = string(raw)
		case "/v2026/configuration-hub/backups":
			body = `[{"jobId":"hub-backup","tenant":"example","status":"COMPLETE","created":"2026-01-01T00:00:00Z","userCanDelete":true,"isPartial":false}]`
		default:
			t.Fatalf("unexpected request %s", r.URL)
		}
		return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}))
	if e != nil {
		t.Fatal(e)
	}
	b := a.Backups(c)
	service := exporter.Service{Engine: exporter.Engine{Config: c, API: a}, Store: exporter.FileStore{Output: c.Output}, Lifecycle: exporter.BackupManager{API: b}}
	result, e := service.Export(context.Background())
	if e != nil || len(result.Manifest.Artifacts) != 2 || result.Cleanup.Limitation == "" {
		t.Fatal(result, e)
	}
	before := len(requests)
	if e = b.Delete(context.Background(), c.Tenant, "hub-backup"); !errors.Is(e, exporter.ErrOwnershipUnavailable) || len(requests) != before {
		t.Fatal("SDK delete issued traffic", e)
	}
}
func TestSDKDoesNotRetryPOSTOrExposeBody(t *testing.T) {
	c := exporter.DefaultConfig()
	c.Tenant = "example"
	posts := 0
	a, e := newTestAdapter(t, c, roundTrip(func(r *http.Request) (*http.Response, error) {
		code := 200
		body := `{"access_token":"token"}`
		if r.URL.Path != "/oauth/token" {
			posts++
			code = 500
			body = `{"message":"secret-sensitive-error"}`
		}
		return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}))
	if e != nil {
		t.Fatal(e)
	}
	_, e = a.Start(context.Background(), []string{"ROLE"})
	if e == nil || strings.Contains(e.Error(), "sensitive") || posts != 1 {
		t.Fatal(posts, e)
	}
}

// Native SDK token acquisition uses http.DefaultTransport rather than its API
// transport. These serial tests intercept both paths without opening a socket.
func newTestAdapter(t *testing.T, c exporter.Config, transport http.RoundTripper) (*Adapter, error) {
	t.Helper()
	c.Backoff = "1ns"
	original := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = original })
	cfg := sailpoint.NewConfiguration(sailpoint.ClientConfiguration{ClientId: "id", ClientSecret: "secret", BaseURL: "https://example.api.identitynow.com", TokenURL: "https://example.api.identitynow.com/oauth/token"})
	return NewWithConfiguration(c, cfg, transport)
}
