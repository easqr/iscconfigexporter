package exporter_test

import (
	"context"
	"encoding/json"
	"github.com/elianderson/iscutilities/configexporter"
	"os"
	"testing"
)

type api struct{ raw json.RawMessage }

func (api) Discover(context.Context) ([]string, error)                  { return []string{"ROLE", "SOURCE"}, nil }
func (api) Start(context.Context, []string) (string, error)             { return "job", nil }
func (api) Status(context.Context, string) (string, error)              { return "COMPLETE", nil }
func (a api) Download(context.Context, string) (json.RawMessage, error) { return a.raw, nil }
func TestExternalConsumer(t *testing.T) {
	raw, e := os.ReadFile("testdata/export.json")
	if e != nil {
		t.Fatal(e)
	}
	c := exporter.DefaultConfig()
	c.Tenant = "example"
	c.BaseURL = "https://example.api.identitynow.com"
	c.Output = t.TempDir()
	var service exporter.Exporter = exporter.Service{Engine: exporter.Engine{Config: c, API: api{raw}}, Store: exporter.FileStore{Output: c.Output}}
	r, e := service.Export(context.Background())
	if e != nil || len(r.Manifest.Artifacts) != 2 {
		t.Fatal(r, e)
	}
}
