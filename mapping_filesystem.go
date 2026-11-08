package exporter

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	snapfmt "gitlab.com/easqrtech/iam/iscconfigexporter/snapshot"
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
		return errors.New("a mapping output directory is required")
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
			return errors.New("mapping output must be outside the export directory and must not contain it; choose a separate directory")
		}
	}
	if _, e := os.Lstat(directory); !errors.Is(e, os.ErrNotExist) {
		return errors.New("the mapping output path already exists or cannot be inspected; choose a new accessible directory")
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
				return "", errors.New("cannot resolve the mapping output path; check parent directories and symbolic links")
			}
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		}
		if !errors.Is(e, os.ErrNotExist) {
			return "", errors.New("cannot inspect parent directories for mapping output; check path permissions")
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", errors.New("cannot resolve the mapping output path; check parent directories and symbolic links")
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
		return out, errors.New("cannot create parent directories for mapping output; check permissions and available disk space")
	}
	r, e := os.OpenRoot(parent)
	if e != nil {
		return out, errors.New("cannot open the parent directory for mapping output; check permissions")
	}
	defer r.Close()
	lock := ".isc-mapping-lock-" + snapfmt.Checksum([]byte(name))[:24]
	if e = r.Mkdir(lock, 0700); e != nil {
		return out, errors.New("cannot lock the mapping output; another retrieval may be running (see docs/MAPPINGS.md before removing a stale lock)")
	}
	defer r.Remove(lock)
	if _, e = r.Lstat(name); !errors.Is(e, os.ErrNotExist) {
		return out, errors.New("the mapping output path already exists or cannot be inspected; choose a new accessible directory")
	}
	stage := ".isc-mappings-stage-" + token()
	if e = r.Mkdir(stage, 0700); e != nil {
		return out, errors.New("cannot create a temporary mapping archive; check permissions and available disk space")
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
		return out, errors.New("cannot write mappings.json; check permissions and available disk space")
	}
	if e = writeSync(r, stage+"/provenance.json", metadata); e != nil {
		return out, errors.New("cannot write provenance.json; check permissions and available disk space")
	}
	if e = syncDir(r, stage); e != nil {
		return out, errors.New("cannot sync the mapping archive to storage; check available disk space and filesystem support")
	}
	if e = ctx.Err(); e != nil {
		return out, e
	}
	if e = r.Rename(stage, name); e != nil {
		return out, errors.New("cannot move the completed mapping archive into place; check the destination and permissions")
	}
	published = true
	out = MappingPublication{Directory: dest, Provenance: snap.Provenance}
	if e = syncDir(r, "."); e != nil {
		return out, errors.New("the mapping archive is visible, but syncing its parent directory failed; verify the archive before retrying")
	}
	return out, nil
}
