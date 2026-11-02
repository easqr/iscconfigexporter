package main

import (
	"bytes"
	"context"
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
