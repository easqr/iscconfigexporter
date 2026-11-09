package sdk

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	exporter "github.com/easqr/iscconfigexporter"
)

const sdkMappingFixture = `[{"objectMappingId":"invented-map","objectType":"ROLE","jsonPath":"$.owner.name","sourceValue":"Invented Owner","targetValue":"Other Invented Owner","enabled":false,"future":{"precise":9007199254740993}}]`

func TestSDKMappingRetrievalUsesSharedClient(t *testing.T) {
	tokens, reads := 0, 0
	a, e := newTestAdapter(t, configForTest(), roundTrip(func(r *http.Request) (*http.Response, error) {
		body := `[]`
		switch r.URL.Path {
		case "/oauth/token":
			tokens++
			body = `{"access_token":"token","expires_in":3600}`
		case "/beta/sp-config/config-objects", "/v2026/configuration-hub/backups":
			reads++
			if r.Method != http.MethodGet {
				t.Fatal("unexpected mutation")
			}
		case "/v2026/configuration-hub/object-mappings/source-example":
			reads++
			if r.Method != http.MethodGet || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "Bearer token" {
				t.Fatal("unexpected mapping request")
			}
			body = sdkMappingFixture
		default:
			t.Fatal("unexpected endpoint")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	}))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.Discover(context.Background()); e != nil {
		t.Fatal(e)
	}
	download, e := a.GetMappings(context.Background(), "source-example")
	if e != nil || tokens != 2 || reads != 2 || !bytes.Equal(download.Data, []byte(sdkMappingFixture)) || download.APIOrigin != "https://example.api.identitynow.com" {
		t.Fatal("mapping request or bytes changed", e, tokens, reads)
	}
	if _, e = a.Backups(configForTest()).List(context.Background(), "example"); e != nil {
		t.Fatal(e)
	}
	s, e := exporter.RetrieveMappings(context.Background(), a, exporter.MappingRequest{SourceOrg: "source-example", TargetTenant: "example"})
	if e != nil || s.Provenance.Count != 1 || tokens != 2 || reads != 4 {
		t.Fatal(s.Provenance, e)
	}
	before := tokens + reads
	if _, e = a.GetMappings(context.Background(), "../invalid"); e == nil || tokens+reads != before {
		t.Fatal("invalid source reached network")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = a.GetMappings(ctx, "source-example"); !errors.Is(e, context.Canceled) || tokens+reads != before {
		t.Fatal("canceled request issued traffic", e)
	}
}
func TestSDKMappingRetriesAndSafeErrors(t *testing.T) {
	for _, code := range []int{403, 429, 500} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			c := configForTest()
			c.Retries = 1
			calls := 0
			a, e := newTestAdapter(t, c, roundTrip(func(r *http.Request) (*http.Response, error) {
				body := `{"access_token":"token"}`
				status := 200
				if r.URL.Path != "/oauth/token" {
					if r.URL.Path != "/v2026/configuration-hub/object-mappings/default" || r.Method != http.MethodGet {
						t.Fatal("unexpected endpoint")
					}
					calls++
					status = code
					body = `{"message":"sensitive-mapping-content"}`
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			}))
			if e != nil {
				t.Fatal(e)
			}
			_, e = a.GetMappings(context.Background(), "default")
			want := 2
			if code == 403 {
				want = 1
			}
			if e == nil || calls != want || strings.Contains(e.Error(), "sensitive-mapping-content") {
				t.Fatal(calls, e)
			}
		})
	}
}
func TestSDKMappingMalformedResponses(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `[{"objectMappingId":"x","objectMappingId":"y"}]`, strings.Replace(sdkMappingFixture, `"enabled":false`, `"enabled":null`, 1)} {
		a, e := newTestAdapter(t, configForTest(), roundTrip(func(r *http.Request) (*http.Response, error) {
			content := body
			if r.URL.Path == "/oauth/token" {
				content = `{"access_token":"token"}`
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(content)), Request: r}, nil
		}))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = a.GetMappings(context.Background(), "source"); e == nil {
			t.Fatal("malformed mapping accepted")
		}
	}
}
