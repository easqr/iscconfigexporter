package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	exporter "github.com/elianderson/iscutilities/configexporter"
	"github.com/elianderson/iscutilities/configexporter/sdk"
	sailpoint "github.com/sailpoint-oss/golang-sdk/v2"
)

type mappingTransport func(*http.Request) (*http.Response, error)

func (f mappingTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestMappingCLI(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "exporter.json")
	exports := filepath.Join(root, "exports")
	os.WriteFile(config, []byte(`{"tenant":"target-example","output":`+mustJSON(exports)+`,"backup":{"enabled":true,"policy":"keep_latest"}}`), 0600)
	calls := 0
	status := 200
	factory := func(c exporter.Config) (*sdk.Adapter, error) {
		return sdk.NewWithConfiguration(c, sailpoint.NewConfiguration(sailpoint.ClientConfiguration{BaseURL: "https://target.example.invalid", Token: "synthetic-token"}), mappingTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.Method != http.MethodGet || r.URL.Path != "/v2026/configuration-hub/object-mappings/source-example" {
				t.Fatal("retrieval invoked export, backup or unexpected action")
			}
			body := `[{"objectMappingId":"invented","objectType":"SOURCE","jsonPath":"$.name","sourceValue":"sensitive-source-value","targetValue":"sensitive-target-value","enabled":false}]`
			if status != 200 {
				body = `{"message":"sensitive-error-body"}`
			}
			return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
		}))
	}
	dest := filepath.Join(root, "mappings")
	args := []string{"-config", config, "-retrieve-mappings", "-mapping-source-org", "source-example", "-mapping-output", dest, "-mapping-export-job", "operator-job"}
	var out, errOut bytes.Buffer
	if e := runWithAdapter(context.Background(), args, &out, &errOut, factory); e != nil {
		t.Fatal(e)
	}
	var publication exporter.MappingPublication
	if e := json.Unmarshal(out.Bytes(), &publication); e != nil {
		t.Fatal(e)
	}
	if calls != 1 || publication.Provenance.TargetTenant != "target-example" || publication.Provenance.SourceOrg != "source-example" || publication.Provenance.ExportJobID != "operator-job" || publication.Provenance.Count != 1 || strings.Contains(out.String(), "sensitive-") {
		t.Fatal("unexpected or sensitive CLI output", out.String())
	}
	if _, e := os.Stat(exports); !os.IsNotExist(e) {
		t.Fatal("configuration export output was touched")
	}
	before := calls
	for _, bad := range [][]string{
		{"-config", config, "-mapping-source-org", "source-example"},
		{"-config", config, "-retrieve-mappings"},
		{"-config", config, "-retrieve-mappings", "-mapping-source-org", "source-example", "-mapping-output", filepath.Join(exports, "mappings")},
		append(append([]string{}, args...), "-cleanup-preview"),
		args,
	} {
		out.Reset()
		if e := runWithAdapter(context.Background(), bad, &out, &errOut, factory); e == nil || calls != before || out.Len() != 0 {
			t.Fatal("invalid mapping options reached network")
		}
	}
	// A permission error returns no successful publication or mapping payload.
	status = 403
	dest = filepath.Join(root, "denied")
	args = []string{"-config", config, "-retrieve-mappings", "-mapping-source-org", "source-example", "-mapping-output", dest}
	out.Reset()
	e := runWithAdapter(context.Background(), args, &out, &errOut, factory)
	if e == nil || strings.Contains(e.Error(), "sensitive-error-body") || out.Len() != 0 || calls != before+1 {
		t.Fatal("unsafe permission error", e)
	}
	if _, e = os.Stat(dest); !os.IsNotExist(e) {
		t.Fatal("failed retrieval published output")
	}
}
func mustJSON(s string) string { b, _ := json.Marshal(s); return string(b) }
func TestMappingCLILocalValidation(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "exporter.json")
	os.WriteFile(config, []byte(`{"tenant":"target-example","output":`+mustJSON(filepath.Join(root, "exports"))+`}`), 0600)
	dest := filepath.Join(root, "new", "mappings")
	var out, errOut bytes.Buffer
	factory := func(exporter.Config) (*sdk.Adapter, error) {
		t.Fatal("validation loaded SDK configuration")
		return nil, nil
	}
	if e := runWithAdapter(context.Background(), []string{"-config", config, "-validate", "-retrieve-mappings", "-mapping-source-org", "default", "-mapping-output", dest}, &out, &errOut, factory); e != nil || out.String() != "configuration valid\n" {
		t.Fatal(e, out.String())
	}
	if _, e := os.Stat(filepath.Dir(dest)); !os.IsNotExist(e) {
		t.Fatal("validation wrote output")
	}
}
