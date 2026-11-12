package snapshot

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestReversibleFilenameEscaping(t *testing.T) {
	for _, tc := range []struct{ raw, escaped string }{
		{"a:b", "a%3Ab"}, {"a%3Ab", "a%253Ab"}, {"A/B\\C", "A%2FB%5CC"},
		{"NUL.txt", "%4EUL.txt"}, {"COM¹", "%43OM¹"}, {"  leading", "%20%20leading"},
		{"a\x00\n\u0085", "a%00%0A%C2%85"}, {"Payroll (US) & Café", "Payroll (US) & Café"},
	} {
		got := EscapeFilename(tc.raw)
		if got != tc.escaped {
			t.Fatalf("%q: got %q, want %q", tc.raw, got, tc.escaped)
		}
		decoded, err := url.PathUnescape(got)
		if err != nil || decoded != tc.raw {
			t.Fatalf("escape lost original value: %q %v", decoded, err)
		}
		if issues := filenameProblems(got); len(issues) != 0 {
			t.Fatal(got, issues)
		}
	}
	for _, name := range []string{"Bad/Name", "a:b", "CON.txt", "%3A", " leading"} {
		p, err := ObjectPathWithOptions("ROLE", "a:b", name, FilenameOptions{Mode: NameFilename, Escape: true, AppendID: true})
		if err != nil || !strings.HasSuffix(p, "--a%3Ab.json") {
			t.Fatal(p, err)
		}
	}
	if _, err := ObjectPathWithOptions("ROLE", "id", "", FilenameOptions{Mode: NameFilename, Escape: true}); err == nil {
		t.Fatal("escaping invented a missing name")
	}
	if _, err := ObjectPathWithOptions("ROLE", "id", strings.Repeat(":", 100), FilenameOptions{Mode: NameFilename, Escape: true}); err == nil {
		t.Fatal("escaping bypassed expanded filename limit")
	}
}

func TestLegacyLiteralNameManifest(t *testing.T) {
	files := namedFiles(t, NameFilename, "Active")
	data := files["ROLE/Active.json"]
	delete(files, "ROLE/Active.json")
	files["ROLE/Active.json"] = data
	var m Manifest
	if err := json.Unmarshal(files["manifest.json"], &m); err != nil {
		t.Fatal(err)
	}
	m.Schema = 2
	m.Artifacts[0].Path = "ROLE/Active.json"
	files["manifest.json"], _ = json.Marshal(m)
	for _, mode := range []Mode{SnapshotMode, TreeMode} {
		in, err := FromFiles(files, Options{Mode: mode})
		if err != nil {
			t.Fatal(mode, err)
		}
		if _, err = Clone(in); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPortableLiteralFilenames(t *testing.T) {
	for _, name := range []string{"Payroll (US) & HR + IT", "O'Brien, Inc.", "hello.", "hello ", ".hidden", "a..b", "Café", strings.Repeat("a", 250)} {
		if _, err := ObjectPathWithMode("ROLE", "id", name, NameFilename); err != nil {
			t.Fatal(name, err)
		}
	}
	for _, name := range []string{"", "a/b", `a\b`, "a:b", "a?b", "a*b", "a|b", "a<b", "a>b", `a"b`, "a\x00b", "a\nb", "CON", "COM¹", "LPT9", "nul.txt", " leading", strings.Repeat("a", 251), strings.Repeat("界", 84)} {
		if _, err := ObjectPathWithMode("ROLE", "id", name, NameFilename); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
}

func namedFiles(t *testing.T, mode FilenameMode, name string) map[string][]byte {
	t.Helper()
	p, err := ObjectPathWithMode("ROLE", "abc", name, mode)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{"version": 1, "self": map[string]any{"type": "ROLE", "id": "abc", "name": name}, "object": map[string]any{}})
	schema := 2
	if mode == NameFilename {
		schema = 5
	}
	m := Manifest{Schema: schema, FilenameMode: mode, Tenant: "example", JobID: "job", Completed: time.Now(), Types: []string{"ROLE"}, Counts: map[string]int{"ROLE": 1}, Artifacts: []Artifact{{Path: p, SHA256: Checksum(b)}}}
	mb, _ := json.Marshal(m)
	return map[string][]byte{"manifest.json": mb, p: b}
}

func TestReaderNamingContract(t *testing.T) {
	for _, mode := range []FilenameMode{IDFilename, NameFilename} {
		files := namedFiles(t, mode, "Example")
		for _, readMode := range []Mode{SnapshotMode, TreeMode} {
			in, err := FromFiles(files, Options{Mode: readMode})
			if err != nil {
				t.Fatal(mode, readMode, err)
			}
			if _, err := Clone(in); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := FromFiles(files, Options{Mode: TreeMode, FilenameMode: HexFilename}); err == nil {
			t.Fatal("accepted conflicting override")
		}
		delete(files, "manifest.json")
		if _, err := FromFiles(files, Options{Mode: TreeMode, Types: []string{"ROLE"}, FilenameMode: mode}); err != nil {
			t.Fatal(err)
		}
		if _, err := FromFiles(files, Options{Mode: TreeMode, Types: []string{"ROLE"}}); err == nil {
			t.Fatal("inferred naming without metadata")
		}
	}
	files := namedFiles(t, NameFilename, "Example")
	b := files["ROLE/Example.json"]
	files["ROLE/Wrong.json"] = b
	delete(files, "ROLE/Example.json")
	if _, err := FromFiles(files, Options{Mode: TreeMode}); err == nil {
		t.Fatal("accepted mismatched name/path")
	}
	files = namedFiles(t, NameFilename, "Example")
	files["ROLE/Another.json"] = []byte(strings.ReplaceAll(string(files["ROLE/Example.json"]), "Example", "Another"))
	if _, err := FromFiles(files, Options{Mode: TreeMode}); err == nil {
		t.Fatal("accepted duplicate identity under two names")
	}
	for _, schema := range []string{`"schema":1`, `"schema":6`} {
		files = namedFiles(t, NameFilename, "Example")
		files["manifest.json"] = []byte(strings.Replace(string(files["manifest.json"]), `"schema":5`, schema, 1))
		if _, err := FromFiles(files, Options{Mode: SnapshotMode}); err == nil {
			t.Fatal("accepted unsupported schema/naming combination")
		}
	}
}
