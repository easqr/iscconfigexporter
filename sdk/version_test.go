package sdk

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	sailpoint "github.com/sailpoint-oss/golang-sdk/v2"
	exporter "gitlab.com/easqrtech/iam/iscconfigexporter"
	"gitlab.com/easqrtech/iam/iscconfigexporter/internal/buildinfo"
)

func TestSDKConsumerBuildIdentity(t *testing.T) {
	c := exporter.DefaultConfig()
	c.Tenant = "example"
	adapter, err := NewWithConfiguration(c, sailpoint.NewConfiguration(sailpoint.ClientConfiguration{BaseURL: "https://example.invalid", Token: "synthetic-token"}), roundTrip(func(r *http.Request) (*http.Response, error) {
		want := "(isc-config-exporter/" + buildinfo.Read().ConsumerVersion() + ")"
		if !strings.Contains(r.UserAgent(), want) {
			t.Errorf("SDK request consumer mismatch: %q, want %q", r.UserAgent(), want)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`[]`)), Request: r}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
}
