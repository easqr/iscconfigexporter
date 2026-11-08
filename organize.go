package exporter

import (
	"encoding/json"
	"errors"
	"fmt"
	snapfmt "gitlab.com/easqrtech/iam/iscconfigexporter/snapshot"
	"sort"
	"strings"
	"time"
)

func portable(s string) bool { return snapfmt.ValidateTypes([]string{s}) == nil }

// Organize is pure: it validates the entire download before returning any artifacts.
// Object array order is preserved; only object map keys are canonicalized.
func Organize(tenant string, d Download, completed time.Time) (snapfmt.Snapshot, error) {
	var out snapfmt.Snapshot
	if !portable(tenant) || d.JobID == "" {
		return out, errors.New("invalid export identity")
	}
	v, e := snapfmt.DecodeJSON(d.Raw)
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
	typePaths := map[string]bool{}
	for _, s := range d.Types {
		if !portable(s) {
			return out, errors.New("unsafe selected type")
		}
		if typePaths[strings.ToLower(s)] {
			return out, errors.New("colliding selected types")
		}
		typePaths[strings.ToLower(s)] = true
		selected[s] = true
	}
	out.Manifest = snapfmt.Manifest{Schema: 1, Tenant: tenant, JobID: d.JobID, Completed: completed.UTC(), Types: append([]string{}, d.Types...), Counts: map[string]int{}, Artifacts: []snapfmt.Artifact{}}
	sort.Strings(out.Manifest.Types)
	for _, s := range d.Types {
		out.Manifest.Counts[s] = 0
	}
	seen := map[string]bool{}
	for _, v := range items {
		obj, ok := v.(map[string]any)
		if !ok {
			return snapfmt.Snapshot{}, errors.New("object envelope is not an object")
		}
		self, ok := obj["self"].(map[string]any)
		if !ok {
			return snapfmt.Snapshot{}, errors.New("missing self metadata")
		}
		typ, _ := self["type"].(string)
		id, _ := self["id"].(string)
		ver, ok := obj["version"].(json.Number)
		if !ok {
			return snapfmt.Snapshot{}, errors.New("missing object version")
		}
		n, e := ver.Int64()
		if e != nil || n < 1 {
			return snapfmt.Snapshot{}, errors.New("invalid object version")
		}
		if _, ok := obj["object"].(map[string]any); !ok {
			return snapfmt.Snapshot{}, errors.New("missing object payload")
		}
		if !selected[typ] {
			return snapfmt.Snapshot{}, errors.New("download contains unselected object type")
		}
		p, e := snapfmt.ObjectPath(typ, id)
		if e != nil {
			return snapfmt.Snapshot{}, e
		}
		key := strings.ToLower(p)
		if seen[key] {
			return snapfmt.Snapshot{}, fmt.Errorf("duplicate or colliding object identity")
		}
		seen[key] = true
		b, e := json.MarshalIndent(obj, "", "  ")
		if e != nil {
			return snapfmt.Snapshot{}, errors.New("cannot serialize object")
		}
		b = append(b, '\n')
		out.Artifacts = append(out.Artifacts, snapfmt.Artifact{Path: p, SHA256: snapfmt.Checksum(b), Data: b})
		out.Manifest.Counts[typ]++
	}
	sort.Slice(out.Artifacts, func(i, j int) bool { return out.Artifacts[i].Path < out.Artifacts[j].Path })
	out.Manifest.Artifacts = append(out.Manifest.Artifacts, out.Artifacts...)
	return out, nil
}
