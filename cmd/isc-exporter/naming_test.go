package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func TestCLIFilenameModesAndCheck(t *testing.T) {
	for _, tc := range []struct {
		name        string
		args        []string
		objectName  string
		check, fail bool
		escape      bool
	}{
		{"default", nil, "Good", false, false, false},
		{"id", []string{"-filename-mode", "id"}, "Bad/Name", false, false, false},
		{"name", []string{"-filename-mode", "name"}, "Good name", false, false, false},
		{"unsafe", []string{"-filename-mode", "name"}, "Bad/Name", false, true, false},
		{"check good", []string{"-check-filenames"}, "Good", true, false, false},
		{"check bad", []string{"-check-filenames"}, "Bad/Name", true, true, false},
		{"escaped name", []string{"-filename-mode", "name"}, "Bad/Name", false, false, true},
		{"escaped check", []string{"-check-filenames"}, "Bad/Name", true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			policy, output := filepath.Join(root, "policy.json"), filepath.Join(root, "exports")
			if err := os.WriteFile(policy, []byte(`{"tenant":"example","filename_escape":`+fmt.Sprint(tc.escape)+`,"output":`+mustJSON(output)+`}`), 0600); err != nil {
				t.Fatal(err)
			}
			factory := func(c exporter.Config) (*sdk.Adapter, error) {
				return sdk.NewWithConfiguration(c, sailpoint.NewConfiguration(sailpoint.ClientConfiguration{BaseURL: "https://example.invalid", Token: "test-token"}), mappingTransport(func(r *http.Request) (*http.Response, error) {
					body, code := "", 200
					switch r.URL.Path {
					case "/beta/sp-config/config-objects":
						body = `[{"objectType":"ROLE","exportable":true}]`
					case "/beta/sp-config/export":
						code = 202
						body = `{"jobId":"job","status":"NOT_STARTED","type":"EXPORT","created":"2026-01-01T00:00:00Z","modified":"2026-01-01T00:00:00Z"}`
					case "/beta/sp-config/export/job":
						body = `{"jobId":"job","status":"COMPLETE","type":"EXPORT","created":"2026-01-01T00:00:00Z","modified":"2026-01-01T00:00:00Z"}`
					case "/beta/sp-config/export/job/download":
						body = `{"tenant":"example","objects":[{"version":1,"self":{"type":"ROLE","id":"abc","name":` + mustJSON(tc.objectName) + `},"object":{}}]}`
					default:
						t.Fatalf("unexpected request %s", r.URL.Path)
					}
					return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
				}))
			}
			var stdout, stderr bytes.Buffer
			err := runWithAdapter(context.Background(), append([]string{"-config", policy}, tc.args...), &stdout, &stderr, factory)
			if (err != nil) != tc.fail {
				t.Fatal(err, stdout.String())
			}
			if tc.check {
				var report exporter.FilenameCheck
				if err := json.Unmarshal(stdout.Bytes(), &report); err != nil || len(report.Modes) != 2 {
					t.Fatal(report, err)
				}
				if report.Modes[1].Safe == tc.fail {
					t.Fatal(report)
				}
			}
			_, statErr := os.Stat(output)
			if tc.check || tc.fail {
				if !os.IsNotExist(statErr) {
					t.Fatal("unexpected output writes", statErr)
				}
			} else if statErr != nil {
				t.Fatal(statErr)
			}
		})
	}
}

func TestCLIFilenameOptionsValidateLocally(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "policy.json")
	if err := os.WriteFile(p, []byte(`{"tenant":"example"}`), 0600); err != nil {
		t.Fatal(err)
	}
	factory := func(exporter.Config) (*sdk.Adapter, error) {
		t.Fatal("local option validation accessed connection")
		return nil, nil
	}
	for _, tc := range []struct {
		args []string
		fail bool
	}{
		{[]string{"-filename-mode", "slug"}, true},
		{[]string{"-filename-mode", ""}, true},
		{[]string{"-check-filenames", "-cleanup-preview"}, true},
		{[]string{"-check-filenames", "-retrieve-mappings"}, true},
		{[]string{"-filename-mode", "name", "-validate"}, false},
		{[]string{"-check-filenames", "-validate"}, false},
	} {
		var out bytes.Buffer
		err := runWithAdapter(context.Background(), append([]string{"-config", p}, tc.args...), &out, &out, factory)
		if (err != nil) != tc.fail {
			t.Fatal(tc.args, err)
		}
	}
}
