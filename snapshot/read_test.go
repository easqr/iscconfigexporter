package snapshot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fixture(t *testing.T) map[string][]byte {
	t.Helper()
	p, _ := ObjectPath("ROLE", "one")
	b := []byte(`{"version":1,"self":{"type":"ROLE","id":"one"},"object":{"id":"one","name":"A","precise":9007199254740993}}`)
	m := Manifest{Schema: 1, Tenant: "invented", JobID: "job", Completed: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Types: []string{"ROLE", "SOURCE"}, Counts: map[string]int{"ROLE": 1, "SOURCE": 0}, Artifacts: []Artifact{{Path: p, SHA256: Checksum(b)}}}
	raw, _ := json.Marshal(m)
	return map[string][]byte{"manifest.json": raw, p: b}
}
func writeFiles(t *testing.T, root string, files map[string][]byte) {
	t.Helper()
	for p, b := range files {
		full := filepath.Join(root, p)
		if e := os.MkdirAll(filepath.Dir(full), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(full, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
}
func TestBoundaryAndTree(t *testing.T) {
	files := fixture(t)
	opts := Options{Mode: SnapshotMode}
	in, e := FromFiles(files, opts)
	if e != nil || len(in.Objects) != 1 {
		t.Fatal(in, e)
	}
	if _, e = Clone(in); e != nil {
		t.Fatal(e)
	}
	for p, b := range files {
		if p != "manifest.json" {
			b[0] = ' '
		}
	}
	if _, e = Clone(in); e != nil {
		t.Fatal("caller byte mutation leaked", e)
	}
	in.Objects[0].Envelope["bad"] = true
	if _, e = Clone(in); e == nil {
		t.Fatal("mutated input accepted")
	}
	files = fixture(t)
	for p := range files {
		if p != "manifest.json" {
			delete(files, p)
		}
	}
	if _, e = FromFiles(files, opts); e == nil {
		t.Fatal("snapshot deletion accepted")
	}
	tree, e := FromFiles(files, Options{Mode: TreeMode})
	if e != nil || len(tree.Objects) != 0 || len(tree.Manifest.Types) != 2 {
		t.Fatal(tree, e)
	}
	delete(files, "manifest.json")
	if _, e = FromFiles(files, Options{Mode: TreeMode}); e == nil {
		t.Fatal("missing scope accepted")
	}
	if _, e = FromFiles(files, Options{Mode: TreeMode, Types: []string{"ROLE"}}); e != nil {
		t.Fatal(e)
	}
	files = fixture(t)
	p, _ := ObjectPath("ROLE", "two")
	files[p] = []byte(`{"version":1,"self":{"type":"ROLE","id":"two"},"object":{"name":"B"}}`)
	if _, e = FromFiles(files, opts); e != nil {
		t.Fatal("unmanaged snapshot files not ignored", e)
	}
	in, e = FromFiles(files, Options{Mode: TreeMode})
	if e != nil || len(in.Objects) != 2 {
		t.Fatal(in, e)
	}
}
func TestInvalidInputs(t *testing.T) {
	cases := map[string]func(map[string][]byte){
		"checksum": func(f map[string][]byte) {
			for p := range f {
				if p != "manifest.json" {
					f[p] = append(f[p], ' ')
				}
			}
		},
		"duplicate": func(f map[string][]byte) {
			for p := range f {
				if p != "manifest.json" {
					f[p] = []byte(`{"version":1,"version":1}`)
				}
			}
		},
		"count": func(f map[string][]byte) {
			var m Manifest
			json.Unmarshal(f["manifest.json"], &m)
			m.Counts["ROLE"] = 2
			f["manifest.json"], _ = json.Marshal(m)
		},
		"path": func(f map[string][]byte) {
			var m Manifest
			json.Unmarshal(f["manifest.json"], &m)
			m.Artifacts[0].Path = "../outside"
			f["manifest.json"], _ = json.Marshal(m)
		},
		"schema": func(f map[string][]byte) {
			var m Manifest
			json.Unmarshal(f["manifest.json"], &m)
			m.Schema = 2
			f["manifest.json"], _ = json.Marshal(m)
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			f := fixture(t)
			change(f)
			if _, e := FromFiles(f, Options{Mode: SnapshotMode}); e == nil {
				t.Fatal("accepted invalid input")
			}
		})
	}
	for _, raw := range []string{`{"a":1,"a":2}`, `{"a":{"b":1,"b":2}}`, `[1] [2]`, `{"a":}`, `null null`} {
		if _, e := DecodeJSON([]byte(raw)); e == nil {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{`{"version":2,"self":{"type":"ROLE","id":"one"},"object":{"name":"A"}}`, `{"version":1,"self":{"type":"SOURCE","id":"one"},"object":{"name":"A"}}`, `{"version":1,"self":{"type":"ROLE","id":"one"},"object":{"id":"other","name":"A"}}`, `{"version":1,"self":{"type":"ROLE","id":"one"},"object":[]}`} {
		f := fixture(t)
		for p := range f {
			if p != "manifest.json" {
				f[p] = []byte(raw)
			}
		}
		if _, e := FromFiles(f, Options{Mode: TreeMode}); e == nil {
			t.Fatal("invalid tree envelope accepted")
		}
	}
}
func TestPinnedRootAndFilesystemSafety(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	writeFiles(t, a, fixture(t))
	writeFiles(t, b, map[string][]byte{"manifest.json": []byte(`{}`)})
	link := filepath.Join(t.TempDir(), "arbitrary")
	if e := os.Symlink(a, link); e != nil {
		t.Fatal(e)
	}
	in, e := read(link, Options{Mode: SnapshotMode}, func() {
		if e := os.Remove(link); e != nil {
			t.Fatal(e)
		}
		if e := os.Symlink(b, link); e != nil {
			t.Fatal(e)
		}
	})
	if e != nil || len(in.Objects) != 1 {
		t.Fatal("root was not pinned", e)
	}
	p, _ := ObjectPath("ROLE", "one")
	if e := os.Remove(filepath.Join(a, p)); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(filepath.Join(b, "manifest.json"), filepath.Join(a, p)); e != nil {
		t.Fatal(e)
	}
	if _, e := Read(a, Options{Mode: SnapshotMode}); e == nil {
		t.Fatal("nested link accepted")
	}
	root := t.TempDir()
	writeFiles(t, root, fixture(t))
	writeFiles(t, root, map[string][]byte{".git/config": []byte("admin"), "notes.txt": []byte("unmanaged")})
	if _, e := Read(root, Options{Mode: TreeMode}); e != nil {
		t.Fatal(e)
	}
	writeFiles(t, root, map[string][]byte{"bad.json": []byte(`{}`)})
	if _, e := Read(root, Options{Mode: TreeMode}); e == nil {
		t.Fatal("malformed unmanaged JSON accepted")
	}
}

func TestCanonicalMetadataAndMemoryScope(t *testing.T) {
	f := fixture(t)
	f["manifest.json"] = []byte(`{"Schema":1,"tenant":"invented","job_id":"job","completed":"2025-01-01T00:00:00Z","types":["ROLE"],"counts":{"ROLE":0},"artifacts":[]}`)
	if _, e := FromFiles(f, Options{Mode: SnapshotMode}); e == nil {
		t.Fatal("manifest property alias accepted")
	}
	for _, p := range []string{"../x", "ROLE/../file.json", "ROLE/id-ZZ.json", "ROLE/id-6F6E65.json", "ROLE/id-.json", "ROLE/id-2e2e.json", "CON/id-61.json"} {
		if ManagedPath(p) {
			t.Fatal(p)
		}
	}
	if _, e := DecodeJSON([]byte{'"', 0xff, '"'}); e == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	f = fixture(t)
	f[".git/unrelated.json"] = []byte(`invalid`)
	if _, e := FromFiles(f, Options{Mode: TreeMode}); e != nil {
		t.Fatal("in-memory administration ignored inconsistently", e)
	}
	if _, e := FromFiles(f, Options{Mode: TreeMode, Types: []string{"ROLE"}}); e == nil {
		t.Fatal("tree manifest scope override accepted")
	}
}
