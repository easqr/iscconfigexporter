package exporter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	snapfmt "github.com/easqr/iscconfigexporter/snapshot"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
)

// FileStore saves read-only snapshot generations and atomically switches the
// tenant symbolic link to the new generation. Use private local POSIX storage
// supporting symlinks, hardlinks, directory sync and atomic rename. Windows,
// network filesystems and unrelated concurrent changes to the root are unsupported.
// Cooperating writers are locked per tenant; old generations are not removed.
type FileStore struct {
	// Output is the private snapshot root directory, created if necessary.
	Output       string
	beforeCommit func() error
}

func validateSnapshot(s snapfmt.Snapshot) error {
	raw := []byte(`{"tenant":`)
	b, _ := json.Marshal(s.Manifest.Tenant)
	raw = append(raw, b...)
	raw = append(raw, []byte(`,"objects":[`)...)
	for i, a := range s.Artifacts {
		if i > 0 {
			raw = append(raw, ',')
		}
		raw = append(raw, a.Data...)
	}
	raw = append(raw, ']', '}')
	v, e := BuildSnapshotWithFilenameOptions(s.Manifest.Tenant, Download{s.Manifest.JobID, s.Manifest.Types, raw}, s.Manifest.Completed, snapfmt.FilenameOptions{Mode: s.Manifest.FilenameMode, Escape: s.Manifest.FilenameEscape, AppendID: s.Manifest.FilenameAppendID})
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(s.Artifacts, v.Artifacts) || !reflect.DeepEqual(s.Manifest.Artifacts, v.Manifest.Artifacts) || !reflect.DeepEqual(s.Manifest.Counts, v.Manifest.Counts) || s.Manifest.Schema != v.Manifest.Schema || s.Manifest.FilenameMode != v.Manifest.FilenameMode || s.Manifest.FilenameEscape != v.Manifest.FilenameEscape || s.Manifest.FilenameAppendID != v.Manifest.FilenameAppendID {
		return errors.New("snapshot metadata or artifacts inconsistent")
	}
	return nil
}
func syncDir(r *os.Root, p string) error {
	f, e := r.Open(p)
	if e != nil {
		return e
	}
	e = f.Sync()
	ce := f.Close()
	return errors.Join(e, ce)
}
func writeSync(r *os.Root, p string, b []byte) error {
	if e := r.MkdirAll(path.Dir(p), 0700); e != nil {
		return e
	}
	f, e := r.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	return errors.Join(e, f.Close())
}
func token() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}

// Publish validates snapshot, writes a complete generation and switches the tenant
// link. The returned manifest includes changes relative to the previous snapshot.
// Unchanged files are hardlinked; keep all generations read-only. Unmanaged regular
// files are carried forward, while nested links, collisions and edited managed
// files prevent publication.
//
// Failures before the link switch preserve the previous snapshot. A failure syncing
// the parent directory after the switch returns the visible manifest and an error;
// verify that generation before retrying. Cancellation racing with the switch can
// still publish. Readers should resolve the current generation once per operation.
func (s FileStore) Publish(ctx context.Context, snap snapfmt.Snapshot) (snapfmt.Manifest, error) {
	var zero snapfmt.Manifest
	if e := ctx.Err(); e != nil {
		return zero, e
	}
	if e := validateSnapshot(snap); e != nil {
		return zero, fmt.Errorf("validate snapshot: %w", e)
	}
	if e := os.MkdirAll(s.Output, 0700); e != nil {
		return zero, e
	}
	r, e := os.OpenRoot(s.Output)
	if e != nil {
		return zero, e
	}
	defer r.Close()
	tenant := snap.Manifest.Tenant
	lock := ".isc-lock-" + tenant
	if e = r.Mkdir(lock, 0700); e != nil {
		return zero, fmt.Errorf("cannot lock the tenant output; another export may be running (see docs/OPERATIONS.md before removing a stale lock): %w", e)
	}
	defer r.Remove(lock)
	if e = r.MkdirAll(".isc-snapshots", 0700); e != nil {
		return zero, e
	}
	info, e := r.Lstat(".isc-snapshots")
	if e != nil || !info.IsDir() {
		return zero, errors.New(".isc-snapshots must be a directory, not a symbolic link or file")
	}
	old := ""
	prior := snapfmt.Manifest{}
	oldFiles := map[string]snapfmt.Artifact{}
	if info, e := r.Lstat(tenant); e == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return zero, errors.New("the tenant output path already exists as a directory or file; choose a fresh output directory or preserve and move the existing path")
		}
		old, e = r.Readlink(tenant)
		if e != nil {
			return zero, e
		}
		prefix := ".isc-snapshots/" + tenant + "-"
		suffix := strings.TrimPrefix(old, prefix)
		if !strings.HasPrefix(old, prefix) || len(suffix) != 32 || strings.Contains(suffix, "/") {
			return zero, errors.New("the tenant symbolic link does not point to a valid exporter generation; inspect the output directory before retrying")
		}
		if _, e = hex.DecodeString(suffix); e != nil {
			return zero, e
		}
		oldInfo, oldErr := r.Lstat(old)
		if oldErr != nil || !oldInfo.IsDir() {
			return zero, errors.New("the previous snapshot generation is missing or is not a directory; inspect the tenant symbolic link")
		}
		previous, e := snapfmt.Read(filepath.Join(s.Output, old), snapfmt.Options{Mode: snapfmt.SnapshotMode})
		if e != nil {
			return zero, fmt.Errorf("previous snapshot is invalid: %w", e)
		}
		prior = previous.Manifest
		if prior.Tenant != tenant {
			return zero, errors.New("the previous snapshot manifest belongs to another tenant; inspect it or choose a fresh output directory")
		}
		for _, a := range prior.Artifacts {
			if !snapfmt.ManagedPathWithMode(a.Path, prior.FilenameMode) || oldFiles[a.Path].Path != "" {
				return zero, errors.New("the previous manifest contains an unsafe or duplicate file path; inspect it or choose a fresh output directory")
			}
			b, e := r.ReadFile(old + "/" + a.Path)
			if e != nil || snapfmt.Checksum(b) != a.SHA256 {
				return zero, errors.New("a file in the previous snapshot was changed or removed; preserve local edits and restore a verified snapshot, or choose a fresh output directory")
			}
			oldFiles[a.Path] = a
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return zero, e
	}
	stage := ".isc-snapshots/" + tenant + "-" + token()
	if e = r.Mkdir(stage, 0700); e != nil {
		return zero, e
	}
	published := false
	defer func() {
		if !published {
			_ = r.RemoveAll(stage)
		}
	}()
	// Copy only unmanaged regular files; never follow nested symlinks or special files.
	if old != "" {
		or, e := r.OpenRoot(old)
		if e != nil {
			return zero, e
		}
		e = fs.WalkDir(or.FS(), ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if p == "." {
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				return errors.New("the previous snapshot contains a symbolic link; only regular files and directories can be carried forward")
			}
			if d.IsDir() {
				return r.MkdirAll(stage+"/"+p, 0700)
			}
			if !d.Type().IsRegular() {
				return errors.New("the previous snapshot contains a special file; only regular files and directories can be carried forward")
			}
			if p == "manifest.json" || oldFiles[p].Path != "" {
				return nil
			}
			for _, a := range snap.Artifacts {
				if snapfmt.CollisionKey(p) == snapfmt.CollisionKey(a.Path) {
					return errors.New("an export file would overwrite an unmanaged local file; preserve and move that file, or choose a fresh output directory")
				}
			}
			b, e := or.ReadFile(p)
			if e != nil {
				return e
			}
			return writeSync(r, stage+"/"+p, b)
		})
		or.Close()
		if e != nil {
			return zero, e
		}
	}
	m := snap.Manifest
	m.Changes = snapfmt.Changes{}
	present := map[string]bool{}
	for _, a := range snap.Artifacts {
		if e = ctx.Err(); e != nil {
			return zero, e
		}
		present[a.Path] = true
		prev, ok := oldFiles[a.Path]
		dest := stage + "/" + a.Path
		if ok && prev.SHA256 == a.SHA256 {
			m.Changes.Unchanged = append(m.Changes.Unchanged, a.Path)
			if e = r.MkdirAll(path.Dir(dest), 0700); e == nil {
				e = r.Link(old+"/"+a.Path, dest)
			}
		} else {
			if ok {
				m.Changes.Modified = append(m.Changes.Modified, a.Path)
			} else {
				m.Changes.Created = append(m.Changes.Created, a.Path)
			}
			e = writeSync(r, dest, a.Data)
		}
		if e != nil {
			return zero, e
		}
	}
	for _, a := range prior.Artifacts {
		if !present[a.Path] {
			m.Changes.Deleted = append(m.Changes.Deleted, a.Path)
		}
	}
	data, e := json.MarshalIndent(m, "", "  ")
	if e != nil {
		return zero, e
	}
	if e = writeSync(r, stage+"/manifest.json", append(data, '\n')); e != nil {
		return zero, e
	}
	// Sync all generation directories before publishing their name.
	sr, e := r.OpenRoot(stage)
	if e != nil {
		return zero, e
	}
	e = fs.WalkDir(sr.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return syncDir(sr, p)
		}
		return nil
	})
	sr.Close()
	if e != nil {
		return zero, e
	}
	if e = syncDir(r, ".isc-snapshots"); e != nil {
		return zero, e
	}
	if s.beforeCommit != nil {
		if e = s.beforeCommit(); e != nil {
			return zero, e
		}
	}
	if e = ctx.Err(); e != nil {
		return zero, e
	}
	pointer := ".isc-pointer-" + token()
	if e = r.Symlink(stage, pointer); e != nil {
		return zero, e
	}
	defer r.Remove(pointer)
	if e = r.Rename(pointer, tenant); e != nil {
		return zero, e
	}
	published = true
	if e = syncDir(r, "."); e != nil {
		return m, fmt.Errorf("the new snapshot is visible, but syncing its directory failed; verify the current snapshot before retrying: %w", e)
	}
	return m, nil
}

// CurrentPath resolves output/tenant once and returns its filesystem path. Reuse
// that path for a coherent multi-file read instead of resolving the tenant link
// for each file. It does not keep a directory handle open, reserve the generation,
// validate its manifest or prevent an operator from deleting it. Only tenant syntax
// and filesystem path resolution are checked; snapshot.Read validates contents.
func CurrentPath(output, tenant string) (string, error) {
	if !portable(tenant) {
		return "", errors.New("invalid tenant")
	}
	return filepath.EvalSymlinks(filepath.Join(output, tenant))
}
