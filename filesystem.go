package exporter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
)

// FileStore publishes immutable generations through an atomic tenant symlink swap.
// Output must be a trusted directory; readers should resolve the symlink once.
type FileStore struct {
	Output       string
	beforeCommit func() error
}

func validateSnapshot(s Snapshot) error {
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
	v, e := Organize(s.Manifest.Tenant, Download{s.Manifest.JobID, s.Manifest.Types, raw}, s.Manifest.Completed)
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(s.Artifacts, v.Artifacts) || !reflect.DeepEqual(s.Manifest.Artifacts, v.Manifest.Artifacts) || !reflect.DeepEqual(s.Manifest.Counts, v.Manifest.Counts) || s.Manifest.Schema != 1 {
		return errors.New("snapshot metadata or artifacts inconsistent")
	}
	return nil
}
func managedPath(p string) bool {
	parts := strings.Split(p, "/")
	if len(parts) != 2 || !portable(parts[0]) || !strings.HasPrefix(parts[1], "id-") || !strings.HasSuffix(parts[1], ".json") {
		return false
	}
	id, e := hex.DecodeString(strings.TrimSuffix(strings.TrimPrefix(parts[1], "id-"), ".json"))
	if e != nil {
		return false
	}
	v, e := objectPath(parts[0], string(id))
	return e == nil && v == p
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
func (s FileStore) Publish(ctx context.Context, snap Snapshot) (Manifest, error) {
	var zero Manifest
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
		return zero, fmt.Errorf("acquire output lock (inspect stale lock after interruption): %w", e)
	}
	defer r.Remove(lock)
	if e = r.MkdirAll(".isc-snapshots", 0700); e != nil {
		return zero, e
	}
	info, e := r.Lstat(".isc-snapshots")
	if e != nil || !info.IsDir() {
		return zero, errors.New("snapshot root must be a real directory")
	}
	old := ""
	prior := Manifest{}
	oldFiles := map[string]Artifact{}
	if info, e := r.Lstat(tenant); e == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return zero, errors.New("tenant output exists and is not an exporter pointer")
		}
		old, e = r.Readlink(tenant)
		if e != nil {
			return zero, e
		}
		prefix := ".isc-snapshots/" + tenant + "-"
		suffix := strings.TrimPrefix(old, prefix)
		if !strings.HasPrefix(old, prefix) || len(suffix) != 32 || strings.Contains(suffix, "/") {
			return zero, errors.New("untrusted snapshot pointer")
		}
		if _, e = hex.DecodeString(suffix); e != nil {
			return zero, e
		}
		data, e := r.ReadFile(old + "/manifest.json")
		if e != nil {
			return zero, e
		}
		if e = json.Unmarshal(data, &prior); e != nil || prior.Tenant != tenant || prior.Schema != 1 {
			return zero, errors.New("invalid previous manifest")
		}
		for _, a := range prior.Artifacts {
			if !managedPath(a.Path) || oldFiles[a.Path].Path != "" {
				return zero, errors.New("invalid previous artifact path")
			}
			b, e := r.ReadFile(old + "/" + a.Path)
			if e != nil || checksum(b) != a.SHA256 {
				return zero, errors.New("previous managed artifact changed or missing")
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
				return errors.New("symlink in previous snapshot")
			}
			if d.IsDir() {
				return r.MkdirAll(stage+"/"+p, 0700)
			}
			if !d.Type().IsRegular() {
				return errors.New("special file in previous snapshot")
			}
			if p == "manifest.json" || oldFiles[p].Path != "" {
				return nil
			}
			for _, a := range snap.Artifacts {
				if strings.EqualFold(p, a.Path) {
					return errors.New("new managed artifact collides with unmanaged file")
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
	m.Changes = Changes{}
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
		return m, fmt.Errorf("snapshot visible but directory durability uncertain: %w", e)
	}
	return m, nil
}

// CurrentPath resolves and pins a generation for coherent multi-file reads.
func CurrentPath(output, tenant string) (string, error) {
	if !portable(tenant) {
		return "", errors.New("invalid tenant")
	}
	return filepath.EvalSymlinks(filepath.Join(output, tenant))
}
