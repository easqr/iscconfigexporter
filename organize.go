package exporter

import (
	"encoding/json"
	"errors"
	"fmt"
	"gitlab.com/easqrtech/iam/iscconfigexporter/internal/envelope"
	snapfmt "gitlab.com/easqrtech/iam/iscconfigexporter/snapshot"
	"sort"
	"strings"
	"time"
)

func portable(s string) bool { return snapfmt.ValidateTypes([]string{s}) == nil }

// BuildSnapshot validates a downloaded export and builds in-memory object files
// and manifest metadata without filesystem or network access. It checks tenant,
// selected types, matching self/payload identities and envelope version 1, preserves
// unknown fields and numeric precision, and rejects duplicate identities or keys.
// Object files are sorted by path; arrays inside each record retain their order.
// JSON map keys are ordered. completed becomes the UTC manifest completion time.
// FileStore.Publish computes changes relative to the previous saved snapshot.
func BuildSnapshot(tenant string, d Download, completed time.Time) (snapfmt.Snapshot, error) {
	var out snapfmt.Snapshot
	if !portable(tenant) || d.JobID == "" {
		return out, errors.New("invalid export identity")
	}
	v, e := snapfmt.DecodeJSON(d.Raw)
	if e != nil {
		return out, errors.New("downloaded export contains invalid JSON or duplicate JSON keys")
	}
	root, ok := v.(map[string]any)
	if !ok {
		return out, errors.New("export must be an object")
	}
	if root["tenant"] != tenant {
		return out, errors.New("downloaded export does not match the policy tenant; check tenant and the SDK connection URL")
	}
	items, ok := root["objects"].([]any)
	if !ok {
		return out, errors.New("downloaded export is missing a valid objects array")
	}
	selected := map[string]bool{}
	typePaths := map[string]bool{}
	for _, s := range d.Types {
		if !portable(s) {
			return out, errors.New("selected export type cannot be used as a safe directory name")
		}
		if typePaths[strings.ToLower(s)] {
			return out, errors.New("selected type names differ only by letter case and would share a directory")
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
			return snapfmt.Snapshot{}, errors.New("an exported configuration record must be a JSON object")
		}
		typ, id, e := envelope.Validate(obj)
		if e != nil {
			return snapfmt.Snapshot{}, fmt.Errorf("invalid exported record: %w", e)
		}
		if !selected[typ] {
			return snapfmt.Snapshot{}, errors.New("downloaded export contains a type outside the requested selection")
		}
		p, e := snapfmt.ObjectPath(typ, id)
		if e != nil {
			return snapfmt.Snapshot{}, e
		}
		key := strings.ToLower(p)
		if seen[key] {
			return snapfmt.Snapshot{}, fmt.Errorf("exported records have duplicate identities or would write to the same file")
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
