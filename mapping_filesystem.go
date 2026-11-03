package exporter

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	snapfmt "github.com/elianderson/iscutilities/configexporter/snapshot"
)

// MappingFileStore publishes an independent bundle to a NEW directory. Its parent
// must be trusted private storage, as for FileStore. Existing bundles are retained.
// ExportRoot optionally enforces separation from a configuration export tree.
type MappingFileStore struct{ Directory, ExportRoot string }
type MappingPublication struct {
	Directory  string            `json:"directory"`
	Provenance MappingProvenance `json:"provenance"`
}

// ValidateMappingDestination performs read-only path checks before authentication
// or retrieval. ExportRoot and the bundle must have disjoint directory trees.
func ValidateMappingDestination(directory, exportRoot string) error {
	if directory == "" {
		return errors.New("mapping output directory is required")
	}
	dest, e := canonicalDestination(directory)
	if e != nil {
		return e
	}
	if exportRoot != "" {
		root, e := canonicalDestination(exportRoot)
		if e != nil {
			return e
		}
		if beneath(dest, root) || beneath(root, dest) {
			return errors.New("mapping output must be separate from the configuration export tree")
		}
	}
	if _, e := os.Lstat(directory); !errors.Is(e, os.ErrNotExist) {
		return errors.New("mapping output directory must not already exist")
	}
	return nil
}
func beneath(child, parent string) bool {
	rel, e := filepath.Rel(parent, child)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
func canonicalDestination(p string) (string, error) {
	abs, e := filepath.Abs(p)
	if e != nil {
		return "", errors.New("invalid mapping output path")
	}
	tail := []string{}
	ancestor := abs
	for {
		_, e := os.Lstat(ancestor)
		if e == nil {
			resolved, e := filepath.EvalSymlinks(ancestor)
			if e != nil {
				return "", errors.New("cannot resolve mapping output ancestry")
			}
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		}
		if !errors.Is(e, os.ErrNotExist) {
			return "", errors.New("cannot inspect mapping output ancestry")
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", errors.New("cannot resolve mapping output ancestry")
		}
		tail = append(tail, filepath.Base(ancestor))
		ancestor = parent
	}
}
func (s MappingFileStore) Publish(ctx context.Context, snap MappingSnapshot) (MappingPublication, error) {
	var out MappingPublication
	snap.Mappings = append(json.RawMessage(nil), snap.Mappings...)
	if e := ctx.Err(); e != nil {
		return out, e
	}
	if e := validateMappingSnapshot(snap); e != nil {
		return out, e
	}
	if e := ValidateMappingDestination(s.Directory, s.ExportRoot); e != nil {
		return out, e
	}
	dest, e := filepath.Abs(s.Directory)
	if e != nil {
		return out, errors.New("invalid mapping output path")
	}
	parent, name := filepath.Dir(dest), filepath.Base(dest)
	if e = os.MkdirAll(parent, 0700); e != nil {
		return out, errors.New("cannot create mapping output parent")
	}
	r, e := os.OpenRoot(parent)
	if e != nil {
		return out, errors.New("cannot open mapping output parent")
	}
	defer r.Close()
	lock := ".isc-mapping-lock-" + snapfmt.Checksum([]byte(name))[:24]
	if e = r.Mkdir(lock, 0700); e != nil {
		return out, errors.New("cannot acquire mapping output lock; inspect any stale lock")
	}
	defer r.Remove(lock)
	if _, e = r.Lstat(name); !errors.Is(e, os.ErrNotExist) {
		return out, errors.New("mapping output directory must not already exist")
	}
	stage := ".isc-mappings-stage-" + token()
	if e = r.Mkdir(stage, 0700); e != nil {
		return out, errors.New("cannot stage mapping bundle")
	}
	published := false
	defer func() {
		if !published {
			_ = r.RemoveAll(stage)
		}
	}()
	metadata, e := json.MarshalIndent(snap.Provenance, "", "  ")
	if e != nil {
		return out, errors.New("cannot encode mapping provenance")
	}
	metadata = append(metadata, '\n')
	if e = writeSync(r, stage+"/mappings.json", snap.Mappings); e != nil {
		return out, errors.New("cannot write mappings")
	}
	if e = writeSync(r, stage+"/provenance.json", metadata); e != nil {
		return out, errors.New("cannot write mapping provenance")
	}
	if e = syncDir(r, stage); e != nil {
		return out, errors.New("cannot sync mapping bundle")
	}
	if e = ctx.Err(); e != nil {
		return out, e
	}
	if e = r.Rename(stage, name); e != nil {
		return out, errors.New("cannot publish mapping bundle")
	}
	published = true
	out = MappingPublication{Directory: dest, Provenance: snap.Provenance}
	if e = syncDir(r, "."); e != nil {
		return out, errors.New("mapping bundle visible but directory durability uncertain")
	}
	return out, nil
}
