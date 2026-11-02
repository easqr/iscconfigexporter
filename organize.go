package exporter

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode"
)

// decodeJSON rejects duplicate keys instead of silently losing configuration data.
func decodeJSON(raw []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	v, e := value(d)
	if e != nil {
		return nil, e
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, errors.New("trailing JSON")
	}
	return v, nil
}
func value(d *json.Decoder) (any, error) {
	t, e := d.Token()
	if e != nil {
		return nil, e
	}
	switch t {
	case json.Delim('{'):
		m := map[string]any{}
		for d.More() {
			k, e := d.Token()
			if e != nil {
				return nil, e
			}
			s, ok := k.(string)
			if !ok {
				return nil, errors.New("invalid key")
			}
			if _, ok = m[s]; ok {
				return nil, errors.New("duplicate JSON key")
			}
			v, e := value(d)
			if e != nil {
				return nil, e
			}
			m[s] = v
		}
		_, e = d.Token()
		return m, e
	case json.Delim('['):
		a := []any{}
		for d.More() {
			v, e := value(d)
			if e != nil {
				return nil, e
			}
			a = append(a, v)
		}
		_, e = d.Token()
		return a, e
	}
	return t, nil
}
func checksum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func portable(s string) bool {
	if !validComponent(s) {
		return false
	}
	u := strings.ToUpper(s)
	switch u {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return false
	}
	return true
}
func objectPath(typ, id string) (string, error) {
	if !portable(typ) || id == "" || len(id) > 100 || id == "." || id == ".." || strings.ContainsAny(id, "/\\") || strings.IndexFunc(id, unicode.IsControl) >= 0 {
		return "", errors.New("unsafe object identity")
	}
	return typ + "/id-" + hex.EncodeToString([]byte(id)) + ".json", nil
}

// Organize is pure: it validates the entire download before returning any artifacts.
// Object array order is preserved; only object map keys are canonicalized.
func Organize(tenant string, d Download, completed time.Time) (Snapshot, error) {
	var out Snapshot
	if !portable(tenant) || d.JobID == "" {
		return out, errors.New("invalid export identity")
	}
	v, e := decodeJSON(d.Raw)
	if e != nil {
		return out, errors.New("invalid export JSON (including duplicate keys)")
	}
	root, ok := v.(map[string]any)
	if !ok {
		return out, errors.New("export must be an object")
	}
	if root["tenant"] != tenant {
		return out, errors.New("download tenant mismatch")
	}
	items, ok := root["objects"].([]any)
	if !ok {
		return out, errors.New("export objects must be an array")
	}
	selected := map[string]bool{}
	for _, s := range d.Types {
		if !portable(s) {
			return out, errors.New("unsafe selected type")
		}
		selected[s] = true
	}
	out.Manifest = Manifest{Schema: 1, Tenant: tenant, JobID: d.JobID, Completed: completed.UTC(), Types: append([]string{}, d.Types...), Counts: map[string]int{}, Artifacts: []Artifact{}}
	sort.Strings(out.Manifest.Types)
	for _, s := range d.Types {
		out.Manifest.Counts[s] = 0
	}
	seen := map[string]bool{}
	for _, v := range items {
		obj, ok := v.(map[string]any)
		if !ok {
			return Snapshot{}, errors.New("object envelope is not an object")
		}
		self, ok := obj["self"].(map[string]any)
		if !ok {
			return Snapshot{}, errors.New("missing self metadata")
		}
		typ, _ := self["type"].(string)
		id, _ := self["id"].(string)
		ver, ok := obj["version"].(json.Number)
		if !ok {
			return Snapshot{}, errors.New("missing object version")
		}
		n, e := ver.Int64()
		if e != nil || n < 1 {
			return Snapshot{}, errors.New("invalid object version")
		}
		if _, ok := obj["object"].(map[string]any); !ok {
			return Snapshot{}, errors.New("missing object payload")
		}
		if !selected[typ] {
			return Snapshot{}, errors.New("download contains unselected object type")
		}
		p, e := objectPath(typ, id)
		if e != nil {
			return Snapshot{}, e
		}
		key := strings.ToLower(p)
		if seen[key] {
			return Snapshot{}, fmt.Errorf("duplicate or colliding object identity")
		}
		seen[key] = true
		b, e := json.MarshalIndent(obj, "", "  ")
		if e != nil {
			return Snapshot{}, errors.New("cannot serialize object")
		}
		b = append(b, '\n')
		out.Artifacts = append(out.Artifacts, Artifact{Path: p, SHA256: checksum(b), Data: b})
		out.Manifest.Counts[typ]++
	}
	sort.Slice(out.Artifacts, func(i, j int) bool { return out.Artifacts[i].Path < out.Artifacts[j].Path })
	out.Manifest.Artifacts = append(out.Manifest.Artifacts, out.Artifacts...)
	return out, nil
}
