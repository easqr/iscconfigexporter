package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	exporter "github.com/easqr/iscconfigexporter"
	"github.com/easqr/iscconfigexporter/sdk"
)

func TestVersionWithoutConfiguration(t *testing.T) {
	t.Chdir(t.TempDir())
	factory := func(exporter.Config) (*sdk.Adapter, error) {
		t.Fatal("version attempted to load SDK configuration")
		return nil, nil
	}
	var out, stderr bytes.Buffer
	if err := runWithAdapter(context.Background(), []string{"-version", "-config", "missing.json"}, &out, &stderr, factory); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"isc-exporter ", "\ncommit: ", "\nmodified: ", "\ngo: ", "\nplatform: "} {
		if !strings.Contains(out.String(), label) {
			t.Fatalf("missing %q in %q", label, out.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatal("version wrote an error", stderr.String())
	}
	sentinel := errors.New("output unavailable")
	if err := runWithAdapter(context.Background(), []string{"-version"}, versionErrorWriter{sentinel}, &stderr, factory); !errors.Is(err, sentinel) {
		t.Fatal("version ignored output failure", err)
	}
}

type versionErrorWriter struct{ err error }

func (w versionErrorWriter) Write([]byte) (int, error) { return 0, w.err }
