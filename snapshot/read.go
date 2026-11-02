package snapshot

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

type Mode string

const (
	SnapshotMode Mode = "snapshot"
	TreeMode     Mode = "tree"
)

type Object struct {
	Type, ID, Path string
	Envelope       map[string]any
}
type Input struct {
	files        map[string][]byte
	options      Options
	Mode         Mode     `json:"mode"`
	Label        string   `json:"label"`
	Environment  string   `json:"environment,omitempty"`
	Manifest     Manifest `json:"manifest"`
	ManifestHash string   `json:"manifest_hash,omitempty"`
	ContentHash  string   `json:"content_hash"`
	Objects      []Object `json:"-"`
}
type Options struct {
	Mode               Mode
	Label, Environment string
	Types              []string
}

// Read resolves the supplied root once, then pins the directory handle. It never
// follows nested links. Concurrent edits to a configuration tree are rejected
// when detected; callers must provide a quiescent tree for a coherent inventory.
func Read(root string, o Options) (Input, error) { return read(root, o, nil) }
func read(root string, o Options, afterPin func()) (Input, error) {
	var zero Input
	resolved, e := filepath.EvalSymlinks(root)
	if e != nil {
		return zero, errors.New("cannot resolve input root")
	}
	r, e := os.OpenRoot(resolved)
	if e != nil {
		return zero, errors.New("cannot open input root")
	}
	defer r.Close()
	if afterPin != nil {
		afterPin()
	}
	files := map[string][]byte{}
	read := func(p string) error {
		parts := strings.Split(p, "/")
		for i := range parts {
			st, e := r.Lstat(strings.Join(parts[:i+1], "/"))
			if e != nil {
				return errors.New("missing input artifact")
			}
			if st.Mode()&os.ModeSymlink != 0 || (i == len(parts)-1 && !st.Mode().IsRegular()) {
				return errors.New("unsafe input artifact")
			}
		}
		st, e := r.Stat(p)
		if e != nil {
			return e
		}
		b, e := r.ReadFile(p)
		if e != nil {
			return e
		}
		after, e := r.Stat(p)
		if e != nil || !os.SameFile(st, after) || st.Size() != after.Size() || !st.ModTime().Equal(after.ModTime()) {
			return errors.New("input changed while reading")
		}
		files[p] = b
		return nil
	}
	if e = read("manifest.json"); e != nil {
		if o.Mode != TreeMode {
			return zero, e
		}
		if _, se := r.Lstat("manifest.json"); !errors.Is(se, os.ErrNotExist) {
			return zero, e
		}
	}
	if o.Mode == SnapshotMode {
		var m Manifest
		if e = decodeManifest(files["manifest.json"], &m); e != nil {
			return zero, e
		}
		for _, a := range m.Artifacts {
			if !ManagedPath(a.Path) {
				return zero, errors.New("unsafe artifact path")
			}
			if e = read(a.Path); e != nil {
				return zero, e
			}
		}
	} else if o.Mode == TreeMode {
		e = fs.WalkDir(r.FS(), ".", func(p string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if p == "." {
				return nil
			}
			if p == ".git" || p == ".hg" || p == ".svn" {
				if d.Type()&os.ModeSymlink != 0 || (!d.IsDir() && !d.Type().IsRegular()) {
					return errors.New("unsafe repository administration entry")
				}
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 || (!d.IsDir() && !d.Type().IsRegular()) {
				return errors.New("nested symlink or special file")
			}
			if !d.IsDir() && strings.HasSuffix(p, ".json") && p != "manifest.json" {
				if !ManagedPath(p) {
					return errors.New("unrecognized JSON configuration path")
				}
				return read(p)
			}
			return nil
		})
		if e != nil {
			return zero, e
		}
	} else {
		return zero, errors.New("explicit input mode required")
	}
	out, e := FromFiles(files, o)
	if e != nil {
		return zero, e
	}
	// Recheck bytes through the pinned handle, including manifest; don't expose payloads.
	for p, b := range files {
		now, e := r.ReadFile(p)
		if e != nil || !bytes.Equal(b, now) {
			return zero, errors.New("input changed while reading")
		}
	}
	if o.Mode == TreeMode {
		current := map[string]bool{}
		e = fs.WalkDir(r.FS(), ".", func(p string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if p == ".git" || p == ".hg" || p == ".svn" {
				if d.Type()&os.ModeSymlink != 0 || (!d.IsDir() && !d.Type().IsRegular()) {
					return errors.New("unsafe repository administration entry")
				}
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 || (!d.IsDir() && !d.Type().IsRegular()) {
				return errors.New("unsafe tree changed during reading")
			}
			if !d.IsDir() && strings.HasSuffix(p, ".json") {
				current[p] = true
			}
			return nil
		})
		if e != nil {
			return zero, e
		}
		for p := range files {
			if !current[p] {
				return zero, errors.New("tree inventory changed")
			}
		}
		for p := range current {
			if _, ok := files[p]; !ok {
				return zero, errors.New("tree inventory changed")
			}
		}
	}

	return out, nil
}
func administrative(p string) bool {
	part := strings.Split(p, "/")[0]
	return part == ".git" || part == ".hg" || part == ".svn"
}
func decodeManifest(b []byte, m *Manifest) error {
	v, e := DecodeJSON(b)
	if e != nil {
		return errors.New("invalid manifest JSON")
	}
	root, ok := v.(map[string]any)
	if !ok {
		return errors.New("manifest object required")
	}
	// Decode exact contract spellings. Unknown metadata remains represented by the
	// original manifest hash; case aliases must not override canonical properties.
	known := map[string]any{}
	for _, k := range []string{"schema", "tenant", "job_id", "completed", "types", "counts", "artifacts", "changes"} {
		if x, ok := root[k]; ok {
			known[k] = x
		}
	}
	if artifacts, ok := known["artifacts"].([]any); ok {
		for _, v := range artifacts {
			a, ok := v.(map[string]any)
			if !ok {
				return errors.New("invalid manifest artifact")
			}
			p, pok := a["path"].(string)
			hash, hok := a["sha256"].(string)
			if !pok || !hok || !ManagedPath(p) || len(hash) != 64 || strings.ToLower(hash) != hash {
				return errors.New("invalid artifact metadata")
			}
			if _, e := hex.DecodeString(hash); e != nil {
				return errors.New("invalid checksum metadata")
			}
		}
	}
	encoded, _ := json.Marshal(known)
	if e = json.Unmarshal(encoded, m); e != nil {
		return errors.New("invalid manifest")
	}
	if m.Schema != 1 || !portable(m.Tenant) || m.JobID == "" || m.Completed.IsZero() {
		return errors.New("unsupported or incomplete manifest")
	}
	if e := ValidateTypes(m.Types); e != nil {
		return e
	}
	if len(m.Counts) != len(m.Types) {
		return errors.New("invalid manifest count scope")
	}
	for _, t := range m.Types {
		n, ok := m.Counts[t]
		if !ok || n < 0 {
			return errors.New("invalid manifest counts")
		}
	}
	return nil
}
func ManagedPath(p string) bool {
	parts := strings.Split(p, "/")
	if len(parts) != 2 || !strings.HasPrefix(parts[1], "id-") || !strings.HasSuffix(parts[1], ".json") {
		return false
	}
	id, e := hex.DecodeString(strings.TrimSuffix(strings.TrimPrefix(parts[1], "id-"), ".json"))
	if e != nil {
		return false
	}
	expected, e := ObjectPath(parts[0], string(id))
	return e == nil && expected == p
}
func ValidateTypes(types []string) error {
	seen := map[string]bool{}
	if len(types) == 0 {
		return errors.New("selected types required")
	}
	for _, t := range types {
		key := strings.ToLower(t)
		if !portable(t) || seen[key] {
			return errors.New("invalid or duplicate selected type")
		}
		seen[key] = true
	}
	return nil
}

// FromFiles copies and validates exporter-format bytes at the same boundary as Read.
// In tree mode, manifest types define scope; without a manifest Types is required.
func FromFiles(files map[string][]byte, o Options) (Input, error) {
	out := Input{Mode: o.Mode, Label: o.Label, Environment: o.Environment}
	fail := func(s string) (Input, error) { return Input{}, errors.New(s) }
	if o.Mode != SnapshotMode && o.Mode != TreeMode {
		return fail("explicit input mode required")
	}
	m := Manifest{}
	if b, ok := files["manifest.json"]; ok {
		if e := decodeManifest(b, &m); e != nil {
			return Input{}, e
		}
		out.ManifestHash = Checksum(b)
	} else if o.Mode == SnapshotMode {
		return fail("manifest required")
	}
	if len(o.Types) > 0 {
		if o.Mode == SnapshotMode {
			return fail("snapshot scope comes from manifest")
		}
		if m.Schema != 0 {
			return fail("tree type override with manifest is ambiguous")
		}
		m.Types = append([]string{}, o.Types...)
	}
	if e := ValidateTypes(m.Types); e != nil {
		return Input{}, e
	}
	m.Types = append([]string{}, m.Types...)
	sort.Strings(m.Types)
	selected := map[string]bool{}
	counts := map[string]int{}
	for _, t := range m.Types {
		selected[t] = true
		counts[t] = 0
	}
	listed := map[string]Artifact{}
	for _, a := range m.Artifacts {
		if !ManagedPath(a.Path) || listed[a.Path].Path != "" {
			return fail("invalid or duplicate artifact path")
		}
		listed[a.Path] = a
	}
	paths := []string{}
	if o.Mode == SnapshotMode {
		for p := range listed {
			paths = append(paths, p)
		}
	} else {
		for p := range files {
			if p != "manifest.json" && !administrative(p) && strings.HasSuffix(p, ".json") {
				paths = append(paths, p)
			}
		}
	}
	sort.Strings(paths)
	seen := map[string]bool{}
	content := []Artifact{}
	for _, p := range paths {
		if !ManagedPath(p) {
			return fail("unsafe artifact path")
		}
		b, ok := files[p]
		if !ok {
			return fail("missing listed artifact")
		}
		hash := Checksum(b)
		if o.Mode == SnapshotMode && listed[p].SHA256 != hash {
			return fail("artifact checksum mismatch")
		}
		v, e := DecodeJSON(b)
		if e != nil {
			return fail("invalid artifact JSON")
		}
		env, ok := v.(map[string]any)
		if !ok {
			return fail("invalid envelope")
		}
		ver, ok := env["version"].(json.Number)
		if !ok || ver.String() != "1" {
			return fail("unsupported envelope version")
		}
		self, ok := env["self"].(map[string]any)
		if !ok {
			return fail("missing self metadata")
		}
		typ, _ := self["type"].(string)
		id, _ := self["id"].(string)
		payload, ok := env["object"].(map[string]any)
		if !ok {
			return fail("missing object payload")
		}
		expected, e := ObjectPath(typ, id)
		if e != nil || expected != p || !selected[typ] {
			return fail("type, path, identity or scope mismatch")
		}
		if pid, ok := payload["id"]; ok && pid != id {
			return fail("conflicting payload identity")
		}
		key := strings.ToLower(p)
		if seen[key] {
			return fail("conflicting physical identity")
		}
		seen[key] = true
		counts[typ]++
		out.Objects = append(out.Objects, Object{typ, id, p, env})
		content = append(content, Artifact{Path: p, SHA256: hash})
	}
	if o.Mode == SnapshotMode {
		if len(m.Counts) != len(counts) {
			return fail("count scope mismatch")
		}
		for t, n := range counts {
			if v, ok := m.Counts[t]; !ok || v != n {
				return fail("artifact count mismatch")
			}
		}
	}
	out.files = map[string][]byte{}
	for p, b := range files {
		out.files[p] = append([]byte(nil), b...)
	}
	out.options = o
	out.Manifest = m
	b, _ := json.Marshal(struct {
		Types     []string
		Artifacts []Artifact
	}{m.Types, content})
	out.ContentHash = Checksum(b)
	return out, nil
}

// Clone validates a detached in-memory Input; callers cannot bypass reader validation.
func Clone(in Input) (Input, error) {
	if in.files == nil {
		return Input{}, errors.New("construct inputs using Read or FromFiles")
	}
	out, e := FromFiles(in.files, in.options)
	if e != nil {
		return Input{}, e
	}
	if !reflect.DeepEqual(out.Objects, in.Objects) || !reflect.DeepEqual(out.Manifest, in.Manifest) || out.ContentHash != in.ContentHash || out.ManifestHash != in.ManifestHash || out.Mode != in.Mode {
		return Input{}, errors.New("validated input was mutated")
	}
	out.Label = in.Label
	out.Environment = in.Environment
	return out, nil
}
