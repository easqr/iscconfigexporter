package exporter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const mappingFixture = `[
 {"objectMappingId":"synthetic-a","objectType":"SOURCE","jsonPath":"$.connectorAttributes.baseDN","sourceValue":"dc=example,dc=com","targetValue":"dc=example-dev,dc=com","enabled":true,"created":"2026-01-01T00:00:00Z","modified":"2026-01-02T00:00:00Z","future":{"precise":9007199254740993}},
 {"objectMappingId":"synthetic-b","objectType":"ROLE","jsonPath":"$..unsupported","sourceValue":"","targetValue":"","enabled":false}
]`

type mappingAPIStub struct {
	calls    int
	source   string
	download MappingDownload
	err      error
}

func (a *mappingAPIStub) GetMappings(ctx context.Context, source string) (MappingDownload, error) {
	a.calls++
	a.source = source
	return a.download, a.err
}
func retrieveFixture(t *testing.T) MappingSnapshot {
	t.Helper()
	a := &mappingAPIStub{download: MappingDownload{Data: []byte(mappingFixture), APIOrigin: "https://target.example.invalid/"}}
	s, e := RetrieveMappings(context.Background(), a, MappingRequest{SourceOrg: "source-example", TargetTenant: "target-example", ExportJobID: "correlation-only"})
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestRetrieveMappingBytesAndProvenance(t *testing.T) {
	a := &mappingAPIStub{download: MappingDownload{Data: []byte(mappingFixture), APIOrigin: "https://target.example.invalid/"}}
	before := time.Now()
	s, e := RetrieveMappings(context.Background(), a, MappingRequest{SourceOrg: "source-example", TargetTenant: "target-example", ExportJobID: "job-example"})
	if e != nil {
		t.Fatal(e)
	}
	if a.calls != 1 || a.source != "source-example" || s.Provenance.Count != 2 || s.Provenance.Schema != 1 || s.Provenance.APIVersion != "v2026" || s.Provenance.APIOrigin != "https://target.example.invalid" || s.Provenance.ExportJobID != "job-example" || s.Provenance.Retrieved.Before(before) || len(s.Provenance.RetrievalID) != 32 || !bytes.Equal(s.Mappings, []byte(mappingFixture)) {
		t.Fatalf("unexpected retrieval provenance: %+v", s.Provenance)
	}
	a.download.Data[0] = ' '
	if !bytes.Equal(s.Mappings, []byte(mappingFixture)) {
		t.Fatal("returned bytes alias the API")
	}
	if e = validateMappingSnapshot(s); e != nil {
		t.Fatal(e)
	}
	a = &mappingAPIStub{download: MappingDownload{Data: []byte(`[]`), APIOrigin: "https://target.example.invalid"}}
	s, e = RetrieveMappings(context.Background(), a, MappingRequest{SourceOrg: "default", TargetTenant: "target-example"})
	if e != nil || s.Provenance.SourceOrg != "default" || s.Provenance.Count != 0 {
		t.Fatal(s.Provenance, e)
	}
}
func TestInvalidMappingResponses(t *testing.T) {
	for _, raw := range []string{`null`, `{}`, `[null]`, `[{"objectMappingId":"x"}]`, strings.Replace(mappingFixture, `"enabled":true`, `"enabled":null`, 1), strings.Replace(mappingFixture, `"sourceValue":"dc=example,dc=com"`, `"sourceValue":123`, 1), strings.Replace(mappingFixture, `"enabled":true`, `"enabled":true,"enabled":false`, 1), strings.Replace(mappingFixture, `"synthetic-b"`, `"synthetic-a"`, 1), strings.Replace(mappingFixture, `"created":"2026-01-01T00:00:00Z"`, `"created":{}`, 1)} {
		if _, e := ValidateMappings([]byte(raw)); e == nil {
			t.Fatal("invalid mapping accepted")
		}
	}
	for _, r := range []MappingRequest{{SourceOrg: "../unsafe", TargetTenant: "target"}, {SourceOrg: "valid"}, {SourceOrg: "valid", TargetTenant: "valid", ExportJobID: "job\n"}} {
		a := &mappingAPIStub{}
		if _, e := RetrieveMappings(context.Background(), a, r); e == nil || a.calls != 0 {
			t.Fatal("invalid request reached API")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a := &mappingAPIStub{}
	if _, e := RetrieveMappings(ctx, a, MappingRequest{SourceOrg: "source", TargetTenant: "target"}); !errors.Is(e, context.Canceled) || a.calls != 0 {
		t.Fatal(e)
	}
	a = &mappingAPIStub{download: MappingDownload{Data: []byte(`[]`), APIOrigin: "http://invalid.example"}}
	if _, e := RetrieveMappings(context.Background(), a, MappingRequest{SourceOrg: "source", TargetTenant: "target"}); e == nil {
		t.Fatal("invalid API origin accepted")
	}
}
func TestMappingPublication(t *testing.T) {
	s := retrieveFixture(t)
	root := t.TempDir()
	dest := filepath.Join(root, "saved-mappings")
	store := MappingFileStore{Directory: dest, ExportRoot: filepath.Join(root, "exports")}
	result, e := store.Publish(context.Background(), s)
	if e != nil || result.Directory != dest {
		t.Fatal(result, e)
	}
	data, e := os.ReadFile(filepath.Join(dest, "mappings.json"))
	if e != nil || !bytes.Equal(data, s.Mappings) {
		t.Fatal("mapping bytes changed", e)
	}
	raw, e := os.ReadFile(filepath.Join(dest, "provenance.json"))
	if e != nil {
		t.Fatal(e)
	}
	var p MappingProvenance
	if e = json.Unmarshal(raw, &p); e != nil || p != s.Provenance {
		t.Fatal(p, e)
	}
	for _, path := range []string{dest, filepath.Join(dest, "mappings.json"), filepath.Join(dest, "provenance.json")} {
		st, e := os.Stat(path)
		if e != nil {
			t.Fatal(e)
		}
		want := os.FileMode(0600)
		if st.IsDir() {
			want = 0700
		}
		if st.Mode().Perm() != want {
			t.Fatal("permissions", st.Mode())
		}
	}
	if _, e = store.Publish(context.Background(), s); e == nil {
		t.Fatal("existing bundle overwritten")
	}
	after, _ := os.ReadFile(filepath.Join(dest, "mappings.json"))
	if !bytes.Equal(after, s.Mappings) {
		t.Fatal("existing bundle changed")
	}
	tampered := s
	tampered.Mappings = []byte(`[]`)
	if _, e = (MappingFileStore{Directory: filepath.Join(root, "bad")}).Publish(context.Background(), tampered); e == nil {
		t.Fatal("inconsistent bundle accepted")
	}
	if _, e = os.Stat(filepath.Join(root, "bad")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("invalid bundle created output")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = (MappingFileStore{Directory: filepath.Join(root, "canceled")}).Publish(ctx, s); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Fatal("staging or lock leaked", len(entries))
	}
}
func TestMappingOutputSeparation(t *testing.T) {
	root := t.TempDir()
	exports := filepath.Join(root, "exports")
	if e := os.Mkdir(exports, 0700); e != nil {
		t.Fatal(e)
	}
	link := filepath.Join(root, "alias")
	if e := os.Symlink(exports, link); e != nil {
		t.Fatal(e)
	}
	for _, dest := range []string{"", exports, filepath.Join(exports, "new"), filepath.Join(link, "new"), root} {
		if e := ValidateMappingDestination(dest, exports); e == nil {
			t.Fatal("unsafe destination accepted", dest)
		}
	}
	for _, dest := range []string{filepath.Join(root, "mapping-bundle"), filepath.Join(root, "new-parent", "bundle")} {
		if e := ValidateMappingDestination(dest, exports); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := (MappingFileStore{Directory: filepath.Join(link, "new"), ExportRoot: exports}).Publish(context.Background(), retrieveFixture(t)); e == nil {
		t.Fatal("snapshot tree modified")
	}
	entries, _ := os.ReadDir(exports)
	if len(entries) != 0 {
		t.Fatal("export contents changed")
	}
}
