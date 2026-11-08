package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hashicorp/go-retryablehttp"
	sailpoint "github.com/sailpoint-oss/golang-sdk/v2"
	exporter "gitlab.com/easqrtech/iam/iscconfigexporter"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func configForTest() exporter.Config {
	c := exporter.DefaultConfig()
	c.Tenant = "example"
	c.Backoff = "1ns"
	return c
}
func isolateConfig(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	for _, k := range []string{"SAIL_BASE_URL", "SAIL_CLIENT_ID", "SAIL_CLIENT_SECRET", "SAIL_NERM_BASE_URL"} {
		t.Setenv(k, "")
	}
}
func TestNativeConfigurationLookup(t *testing.T) {
	for _, source := range []string{"file", "environment", "environment precedence", "missing", "malformed", "legacy ignored", "incomplete environment", "deprecated home ignored"} {
		t.Run(source, func(t *testing.T) {
			isolateConfig(t)
			if source == "file" || source == "environment precedence" {
				if e := os.WriteFile("config.json", []byte(`{"ClientId":"file-id","ClientSecret":"file-secret","BaseURL":"https://file.api.identitynow.com"}`), 0600); e != nil {
					t.Fatal(e)
				}
			}
			if source == "environment" || source == "environment precedence" {
				t.Setenv("SAIL_BASE_URL", "https://env.api.identitynow.com")
				t.Setenv("SAIL_CLIENT_ID", "env-id")
				t.Setenv("SAIL_CLIENT_SECRET", "env-secret")
			}
			if source == "malformed" {
				if e := os.WriteFile("config.json", []byte(`{"ClientSecret":"sensitive-placeholder",`), 0600); e != nil {
					t.Fatal(e)
				}
			}
			if source == "legacy ignored" {
				t.Setenv("ISC_CLIENT_ID", "legacy-id")
				t.Setenv("ISC_CLIENT_SECRET", "legacy-secret")
			}
			if source == "incomplete environment" {
				t.Setenv("SAIL_BASE_URL", "https://example.api.identitynow.com")
			}
			if source == "deprecated home ignored" {
				directory := filepath.Join(os.Getenv("HOME"), ".sailpoint")
				if e := os.MkdirAll(directory, 0700); e != nil {
					t.Fatal(e)
				}
				legacy := "activeenvironment: example\nenvironments:\n  example:\n    baseurl: https://legacy.api.identitynow.com\n    pat:\n      clientid: legacy-id\n      clientsecret: sensitive-placeholder\n"
				if e := os.WriteFile(filepath.Join(directory, "config.yaml"), []byte(legacy), 0600); e != nil {
					t.Fatal(e)
				}
			}
			a, e := New(configForTest(), nil)
			if source == "missing" || source == "malformed" || source == "legacy ignored" || source == "incomplete environment" || source == "deprecated home ignored" {
				if e == nil || strings.Contains(e.Error(), "sensitive-placeholder") {
					t.Fatal("missing or unsafe error", e)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			// Native configuration reaches the generated API without a second credential source.
			cfg := a.client.Beta.GetConfig()
			expected := "file-id"
			if source != "file" {
				expected = "env-id"
			}
			if cfg.ClientId != expected || cfg.ClientSecret == "" {
				t.Fatal("native configuration not propagated")
			}
		})
	}
}
func TestNativeTokenAndConcurrency(t *testing.T) {
	c := configForTest()
	tokens, reads := 0, 0
	transport := roundTrip(func(r *http.Request) (*http.Response, error) {
		body := `[]`
		if r.URL.Path == "/oauth/token" {
			tokens++
			if e := r.ParseForm(); e != nil {
				t.Fatal(e)
			}
			if r.Form.Get("client_id") != "id" || r.Form.Get("client_secret") != "secret" || r.Form.Get("grant_type") != "client_credentials" {
				t.Fatal("native token payload")
			}
			body = `{"access_token":"native-token","expires_in":3600}`
		} else {
			reads++
			if r.Header.Get("Authorization") != "Bearer native-token" {
				t.Error("native authorization missing")
			}
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})
	a, e := newTestAdapter(t, c, transport)
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := a.Discover(context.Background()); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if tokens != 1 || reads != 8 {
		t.Fatal("SDK cache not reused", tokens, reads)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := a.Discover(ctx); e != context.Canceled {
		t.Fatal(e)
	}
}
func TestSDKReadRetries(t *testing.T) {
	for _, code := range []int{408, 429, 500, 403} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			calls := 0
			c := configForTest()
			c.Retries = 2
			a, e := newTestAdapter(t, c, roundTrip(func(r *http.Request) (*http.Response, error) {
				status := 200
				body := `{"access_token":"token"}`
				if r.URL.Path != "/oauth/token" {
					calls++
					status = code
					body = `{"message":"sensitive-placeholder"}`
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			}))
			if e != nil {
				t.Fatal(e)
			}
			_, e = a.Discover(context.Background())
			want := 3
			if code == 403 {
				want = 1
			}
			if e == nil || calls != want || strings.Contains(e.Error(), "sensitive-placeholder") {
				t.Fatal(calls, want, e)
			}
		})
	}
}
func TestNativeConfigurationIsNotMutated(t *testing.T) {
	cfg := sailpoint.NewConfiguration(sailpoint.ClientConfiguration{BaseURL: "https://example.api.identitynow.com", Token: "existing-token"})
	cfg.Debug = true
	before, e := json.Marshal(cfg)
	if e != nil {
		t.Fatal(e)
	}
	a, e := NewWithConfiguration(configForTest(), cfg, nil)
	if e != nil {
		t.Fatal(e)
	}
	after, _ := json.Marshal(cfg)
	if string(before) != string(after) || a.client.Beta.GetConfig().Token != "existing-token" {
		t.Fatal("caller configuration mutated or token dropped")
	}
}
func TestNativeConnectionValidation(t *testing.T) {
	for _, conn := range []sailpoint.ClientConfiguration{{BaseURL: "http://example", Token: "token"}, {BaseURL: "https://example"}, {BaseURL: "https://example", ClientId: "id", ClientSecret: "secret", TokenURL: "http://example/token"}} {
		if _, e := NewWithConfiguration(configForTest(), sailpoint.NewConfiguration(conn), nil); e == nil {
			t.Fatal("invalid connection accepted")
		}
	}
}

func TestSDKRetryPolicyAndBackoff(t *testing.T) {
	cfg := sailpoint.NewConfiguration(sailpoint.ClientConfiguration{BaseURL: "https://example.api.identitynow.com", Token: "token"})
	a, e := NewWithConfiguration(configForTest(), cfg, nil)
	if e != nil {
		t.Fatal(e)
	}
	r := a.client.Beta.GetConfig().HTTPClient
	resp := &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": []string{"3"}}, Request: &http.Request{Method: http.MethodGet}}
	if d := r.Backoff(r.RetryWaitMin, r.RetryWaitMax, 0, resp); d != 3*time.Second {
		t.Fatal("SDK Retry-After backoff not used", d)
	}
	resp.Request.Method = http.MethodPost
	if retry, e := r.CheckRetry(context.Background(), resp, nil); e != nil || retry {
		t.Fatal("POST eligible for retry", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if retry, e := r.CheckRetry(ctx, resp, nil); e != context.Canceled || retry {
		t.Fatal("retry ignored cancellation", e)
	}
}

func TestNativeHTTPClientNotMutated(t *testing.T) {
	c := configForTest()
	cfg := sailpoint.NewConfiguration(sailpoint.ClientConfiguration{BaseURL: "https://example.api.identitynow.com", Token: "token"})
	original := retryablehttp.NewClient()
	original.RetryMax = 9
	original.HTTPClient.Timeout = 7 * time.Second
	cfg.HTTPClient = original
	a, e := NewWithConfiguration(c, cfg, nil)
	if e != nil {
		t.Fatal(e)
	}
	actual := a.client.Beta.GetConfig().HTTPClient
	if actual == original || actual.HTTPClient == original.HTTPClient || original.RetryMax != 9 || original.HTTPClient.Timeout != 7*time.Second || actual.HTTPClient.Timeout != 7*time.Second {
		t.Fatal("caller HTTP client mutated or not safely isolated")
	}
}
