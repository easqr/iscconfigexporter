package sdk

import (
	"context"
	exporter "github.com/elianderson/iscutilities/configexporter"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestSDKRequests(t *testing.T) {
	c := exporter.DefaultConfig()
	c.Tenant = "example"
	c.BaseURL = "https://example.api.identitynow.com"
	posts := 0
	a, e := New(c, exporter.Credentials{ClientID: "id", ClientSecret: "secret"}, roundTrip(func(r *http.Request) (*http.Response, error) {
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
