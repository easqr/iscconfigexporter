package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	exporter "github.com/easqr/iscconfigexporter"
	"github.com/easqr/iscconfigexporter/sdk"
	sailpoint "github.com/sailpoint-oss/golang-sdk/v2"
)

func TestCLIExportFailureJobContext(t *testing.T) {
	for _, stage := range []string{"start", "poll", "download", "prepare", "publish", "cancel"} {
		t.Run(stage, func(t *testing.T) {
			root := t.TempDir()
			policy := filepath.Join(root, "exporter.json")
			output := filepath.Join(root, "exports")
			if err := os.WriteFile(policy, []byte(`{"tenant":"example","retries":0,"output":`+mustJSON(output)+`}`), 0600); err != nil {
				t.Fatal(err)
			}
			if stage == "publish" {
				if err := os.WriteFile(output, []byte("occupied"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			factory := func(c exporter.Config) (*sdk.Adapter, error) {
				return sdk.NewWithConfiguration(c, sailpoint.NewConfiguration(sailpoint.ClientConfiguration{BaseURL: "https://example.invalid", Token: "sensitive-token"}), mappingTransport(func(r *http.Request) (*http.Response, error) {
					body, code := "", 200
					switch r.URL.Path {
					case "/beta/sp-config/config-objects":
						body = `[{"objectType":"ROLE","exportable":true}]`
					case "/beta/sp-config/export":
						code = 202
						body = `{"jobId":"known-job","status":"NOT_STARTED","type":"EXPORT","created":"2026-01-01T00:00:00Z","modified":"2026-01-01T00:00:00Z"}`
						if stage == "start" {
							code = 403
						}
					case "/beta/sp-config/export/known-job":
						body = `{"jobId":"known-job","status":"COMPLETE","type":"EXPORT","created":"2026-01-01T00:00:00Z","modified":"2026-01-01T00:00:00Z"}`
						if stage == "poll" {
							code = 403
						}
						if stage == "cancel" {
							cancel()
							return nil, r.Context().Err()
						}
					case "/beta/sp-config/export/known-job/download":
						body = `{"tenant":"example","objects":[]}`
						if stage == "download" {
							code = 403
						}
						if stage == "prepare" {
							body = `{"tenant":"other","objects":[]}`
						}
					default:
						t.Fatalf("unexpected request: %s", r.URL.Path)
					}
					if code == 403 {
						body = `{"message":"sensitive-response-body"}`
					}
					return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
				}))
			}
			var out, stderr bytes.Buffer
			err := runWithAdapter(ctx, []string{"-config", policy}, &out, &stderr, factory)
			if err == nil || out.Len() != 0 {
				t.Fatal("failed export reported success", err, out.String())
			}
			var failure *exporter.ExportError
			known := stage != "start"
			if errors.As(err, &failure) != known {
				t.Fatal("incorrect failure job context", err)
			}
			message := cliErrorMessage(err)
			if strings.Contains(message, "known-job") != known {
				t.Fatal("missing or invented job ID", message)
			}
			if stage == "cancel" && (!errors.Is(err, context.Canceled) || !strings.Contains(message, "remote job may continue")) {
				t.Fatal("cancellation lost its cause or job context", message)
			}
			if strings.Contains(message+out.String()+stderr.String(), "sensitive-") {
				t.Fatal("sensitive data in diagnostics")
			}
		})
	}
}
