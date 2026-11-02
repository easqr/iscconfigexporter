package exporter

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func snapshot(t *testing.T, items string) Snapshot {
	t.Helper()
	s, e := Organize("example", download(items), time.Now())
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestPublication(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := FileStore{Output: dir}
	snap := snapshot(t, role)
	m, e := store.Publish(ctx, snap)
	if e != nil || len(m.Changes.Created) != 1 {
		t.Fatal(m, e)
	}
	old, _ := CurrentPath(dir, "example")
	p := filepath.Join(old, snap.Artifacts[0].Path)
	before, _ := os.Stat(p)
	if e = os.WriteFile(filepath.Join(old, "user.txt"), []byte("keep"), 0600); e != nil {
		t.Fatal(e)
	}
	m, e = store.Publish(ctx, snap)
	if e != nil || len(m.Changes.Unchanged) != 1 {
		t.Fatal(m, e)
	}
	current, _ := CurrentPath(dir, "example")
	after, _ := os.Stat(filepath.Join(current, snap.Artifacts[0].Path))
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("unchanged rewritten")
	}
	store.beforeCommit = func() error { return errors.New("injected interruption") }
	if _, e = store.Publish(ctx, snapshot(t, "")); e == nil {
		t.Fatal("failure ignored")
	}
	got, _ := CurrentPath(dir, "example")
	if got != current {
		t.Fatal("failed publication changed output")
	}
	store.beforeCommit = nil
	changed := snapshot(t, strings.Replace(role, "Role", "Changed", 1))
	m, e = store.Publish(ctx, changed)
	if e != nil || len(m.Changes.Modified) != 1 {
		t.Fatal(m, e)
	}
	m, e = store.Publish(ctx, snapshot(t, ""))
	if e != nil || len(m.Changes.Deleted) != 1 {
		t.Fatal(m, e)
	}
	current, _ = CurrentPath(dir, "example")
	if b, e := os.ReadFile(filepath.Join(current, "user.txt")); e != nil || string(b) != "keep" {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(current, snap.Artifacts[0].Path)); !errors.Is(e, os.ErrNotExist) {
		t.Fatal(e)
	}
}
func TestPublicationSafety(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []string{"symlink", "tamper", "lock", "unmanaged"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			store := FileStore{Output: dir}
			snap := snapshot(t, role)
			if _, e := store.Publish(ctx, snap); e != nil {
				t.Fatal(e)
			}
			old, _ := CurrentPath(dir, "example")
			switch kind {
			case "symlink":
				if e := os.Symlink(t.TempDir(), filepath.Join(old, "escape")); e != nil {
					t.Fatal(e)
				}
			case "tamper":
				snap.Artifacts[0].Path = "../escape"
			case "lock":
				if e := os.Mkdir(filepath.Join(dir, ".isc-lock-example"), 0700); e != nil {
					t.Fatal(e)
				}
			case "unmanaged":
				if e := os.WriteFile(filepath.Join(old, "ROLE", "id-6f74686572.json"), []byte("user"), 0600); e != nil {
					t.Fatal(e)
				}
				snap = snapshot(t, strings.Replace(role, "a:b", "other", 1))
			}
			if _, e := store.Publish(ctx, snap); e == nil {
				t.Fatal("unsafe publication succeeded")
			}
			current, _ := CurrentPath(dir, "example")
			if current != old {
				t.Fatal("changed previous generation")
			}
		})
	}
}
