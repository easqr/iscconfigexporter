package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	exporter "github.com/easqr/iscconfigexporter"
	"github.com/easqr/iscconfigexporter/sdk"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI(t *testing.T) {
	t.Setenv("SAIL_BASE_URL", "")
	t.Setenv("SAIL_CLIENT_ID", "")
	t.Setenv("SAIL_CLIENT_SECRET", "")
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if e := os.WriteFile(p, []byte(`{"tenant":"example"}`), 0600); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		args    []string
		wantErr bool
	}{{[]string{"-help"}, false}, {[]string{"-config", p, "-validate"}, false}, {[]string{"-config", p}, true}, {[]string{"-config", p, "extra"}, true}, {[]string{"-unknown"}, true}, {[]string{"-config", filepath.Join(dir, "missing")}, true}} {
		var out, stderr bytes.Buffer
		e := run(context.Background(), tc.args, &out, &stderr)
		if (e != nil) != tc.wantErr {
			t.Fatal(tc.args, e)
		}
		if strings.Contains(out.String(), "secret") {
			t.Fatal("secret in output")
		}
	}
}

func TestLocalValidationDoesNotReadSDKConfig(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	if e := os.WriteFile("exporter.json", []byte(`{"tenant":"example"}`), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile("config.json", []byte(`{"ClientSecret":"sensitive-placeholder",`), 0600); e != nil {
		t.Fatal(e)
	}
	var out, stderr bytes.Buffer
	if e := run(context.Background(), []string{"-validate"}, &out, &stderr); e != nil {
		t.Fatal(e)
	}
	if out.String() != "configuration valid\n" || stderr.Len() != 0 {
		t.Fatal(out.String(), stderr.String())
	}
}

func TestCLIHelpAndOptionErrors(t *testing.T) {
	factory := func(exporter.Config) (*sdk.Adapter, error) {
		t.Fatal("help or invalid options loaded SDK settings")
		return nil, nil
	}
	for _, option := range []string{"-help", "-h"} {
		var out, errOut bytes.Buffer
		if err := runWithAdapter(context.Background(), []string{option}, &out, &errOut, factory); err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{"Usage:", "Examples:", "separate config.json", "still writes an export", "cannot delete backups", "does not verify API access", "-mapping-output", "not verified"} {
			if !strings.Contains(errOut.String(), text) {
				t.Errorf("%s help missing %q", option, text)
			}
		}
		if out.Len() != 0 {
			t.Fatal("help wrote to JSON output")
		}
	}
	for _, args := range [][]string{{"-unknown"}, {"-config"}, {"-validate=invalid"}, {"extra"}} {
		var out, errOut bytes.Buffer
		err := runWithAdapter(context.Background(), args, &out, &errOut, factory)
		if err == nil || !strings.Contains(err.Error(), "-help") {
			t.Fatalf("missing usage guidance for %v: %v", args, err)
		}
		if out.Len() != 0 || errOut.Len() != 0 {
			t.Fatal("option parsing printed a duplicate error or usage dump")
		}
	}
}

func TestCLIPolicyAndMappingErrors(t *testing.T) {
	root := t.TempDir()
	policy := filepath.Join(root, "exporter.json")
	factory := func(exporter.Config) (*sdk.Adapter, error) {
		t.Fatal("invalid settings reached SDK configuration")
		return nil, nil
	}
	for _, tc := range []struct {
		name, policy, want string
		options            []string
	}{
		{"duration", `{"tenant":"example","poll_interval":"0s"}`, "poll_interval must be a positive duration", nil},
		{"log level", `{"tenant":"example","log_level":"verbose"}`, "log_level must be", nil},
		{"backup policy", `{"tenant":"example","backup":{"enabled":true}}`, "backup.policy", nil},
		{"wrong type", `{"tenant":"example","retries":"sensitive-placeholder"}`, `field "retries" has the wrong JSON type`, nil},
		{"unknown field", `{"tenant":"example","ClientSecret":"sensitive-placeholder"}`, "supported fields", nil},
		{"incomplete JSON", `{"tenant":"sensitive-placeholder"`, "JSON is incomplete", nil},
		{"empty file", ``, "policy is empty", nil},
		{"source required", `{"tenant":"example"}`, "requires -mapping-source-org", []string{"-retrieve-mappings"}},
		{"destination required", `{"tenant":"example"}`, "requires -mapping-output", []string{"-retrieve-mappings", "-mapping-source-org", "default"}},
		{"conflicting actions", `{"tenant":"example"}`, "cannot be used together", []string{"-retrieve-mappings", "-cleanup-preview"}},
		{"mapping action required", `{"tenant":"example"}`, "require -retrieve-mappings", []string{"-mapping-export-job", "job"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(policy, []byte(tc.policy), 0600); err != nil {
				t.Fatal(err)
			}
			args := append([]string{"-config", policy, "-validate"}, tc.options...)
			var out, errOut bytes.Buffer
			err := runWithAdapter(context.Background(), args, &out, &errOut, factory)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted %q, got %v", tc.want, err)
			}
			if strings.Contains(err.Error(), "sensitive-placeholder") || out.Len() != 0 || errOut.Len() != 0 {
				t.Fatal("unsafe or duplicate error output")
			}
		})
	}
}

func TestCLIWrappedCancellationMessages(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{fmt.Errorf("cannot download: %w", context.Canceled), "operation canceled"},
		{fmt.Errorf("cannot check progress: %w", context.DeadlineExceeded), "operation timed out"},
	} {
		if got := cliErrorMessage(tc.err); !strings.HasPrefix(got, tc.want) {
			t.Fatalf("unexpected error message: %s", got)
		}
	}
}

func TestCLIJobFailureMessages(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded, errors.New("ISC request failed (HTTP 403)")} {
		err := fmt.Errorf("outer workflow: %w", &exporter.ExportError{JobID: "known-job", Err: fmt.Errorf("request failed: %w", cause)})
		message := cliErrorMessage(err)
		for _, text := range []string{`export job "known-job"`, "inspect", "ISC", "before retrying"} {
			if !strings.Contains(message, text) {
				t.Fatalf("missing %q in %q", text, message)
			}
		}
		if !errors.Is(err, cause) {
			t.Fatal("formatting changed the error chain")
		}
		if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
			if !strings.Contains(message, "remote job may continue") {
				t.Fatal("missing remote cancellation guidance", message)
			}
		} else if !strings.Contains(message, "HTTP 403") {
			t.Fatal("lost failure reason", message)
		}
	}
	if got := cliErrorMessage(context.Canceled); strings.Contains(got, "export job") {
		t.Fatal("invented a job ID", got)
	}
	message := cliErrorMessage(&exporter.ExportError{JobID: "job\nforged log", Err: errors.New("request failed")})
	if strings.Contains(message, "\n") || !strings.Contains(message, `job\nforged log`) {
		t.Fatal("job ID was not safely quoted", message)
	}
}
