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
	t.Setenv("ISC_CLIENT_ID", "")
	t.Setenv("ISC_CLIENT_SECRET", "")
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if e := os.WriteFile(p, []byte(`{"tenant":"example","base_url":"https://example.api.identitynow.com"}`), 0600); e != nil {
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
