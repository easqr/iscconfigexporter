package exporter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	snapfmt "gitlab.com/easqrtech/iam/iscconfigexporter/snapshot"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type cleanupFake struct {
	calls int
	err   error
}

func (f *cleanupFake) Cleanup(context.Context, string, string, BackupConfig) (CleanupReport, error) {
	f.calls++
	return CleanupReport{}, f.err
}
func TestComponentRecovery(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	c := testConfig()
	c.Output = dir
	api := &fakeAPI{raw: download(role).Raw}
	cleanup := &cleanupFake{err: errors.New("cleanup failure")}
	s := Service{Engine: Engine{c, api}, Store: FileStore{Output: dir}, Lifecycle: cleanup}
	result, e := s.Export(ctx)
	if e != nil || result.CleanupError == "" || cleanup.calls != 1 {
		t.Fatal(result, e)
	}
	p, _ := CurrentPath(dir, c.Tenant)
	api.raw = json.RawMessage(`{"tenant":"example","objects":null}`)
	if _, e = s.Export(ctx); e == nil {
		t.Fatal("invalid export published")
	}
	current, _ := CurrentPath(dir, c.Tenant)
	if p != current || cleanup.calls != 1 {
		t.Fatal("invalid export changed output or ran cleanup")
	}
	api.raw = download(role).Raw
	result, e = s.Export(ctx)
	if e != nil || len(result.Manifest.Changes.Unchanged) != 1 {
		t.Fatal(result, e)
	}
	api.raw = download("").Raw
	result, e = s.Export(ctx)
	if e != nil || len(result.Manifest.Changes.Deleted) != 1 {
		t.Fatal(result, e)
	}
	data, e := os.ReadFile(filepath.Join(dir, c.Tenant, "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	var manifest snapfmt.Manifest
	if e = json.Unmarshal(data, &manifest); e != nil || len(manifest.Artifacts) != 0 {
		t.Fatal(manifest, e)
	}
}
func TestLargeAndMultipleTypes(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 3000; i++ {
		if i != 0 {
			b.WriteByte(',')
		}
		typ := "ROLE"
		if i%2 == 0 {
			typ = "SOURCE"
		}
		fmt.Fprintf(&b, `{"version":1,"self":{"type":%q,"id":%q},"object":{"nested":{"futureField":[true,null,1]}}}`, typ, fmt.Sprintf("id-%d", i))
	}
	s, e := BuildSnapshot("example", download(b.String()), snapshot(t, "").Manifest.Completed)
	if e != nil || len(s.Artifacts) != 3000 || s.Manifest.Counts["SOURCE"] != 1500 {
		t.Fatal(len(s.Artifacts), e)
	}
}
func TestConcurrentPublish(t *testing.T) {
	store := FileStore{Output: t.TempDir()}
	s := snapshot(t, role)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := store.Publish(context.Background(), s); errs <- e }()
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		}
	}
	if success < 1 {
		t.Fatal("no publisher succeeded")
	}
	p, e := CurrentPath(store.Output, "example")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.ReadFile(filepath.Join(p, "manifest.json")); e != nil {
		t.Fatal(e)
	}
}
func TestStagingWriteFailure(t *testing.T) {
	dir := t.TempDir()
	store := FileStore{Output: dir}
	ctx := context.Background()
	s := snapshot(t, role)
	if _, e := store.Publish(ctx, s); e != nil {
		t.Fatal(e)
	}
	old, _ := CurrentPath(dir, "example")
	next := snapshot(t, strings.Replace(role, "a:b", "different", 1))
	if e := os.Mkdir(filepath.Join(old, next.Artifacts[0].Path), 0700); e != nil {
		t.Fatal(e)
	}
	if _, e := store.Publish(ctx, next); e == nil {
		t.Fatal("stage write failure ignored")
	}
	current, _ := CurrentPath(dir, "example")
	if current != old {
		t.Fatal("failed staging changed output")
	}
}
