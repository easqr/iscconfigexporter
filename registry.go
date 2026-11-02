package exporter

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// FileRegistry is a trusted local ledger, not independent proof of remote ownership.
// Callers must record reliable creation evidence, never inferred names/requesters.
type FileRegistry struct{ Path string }

func (r FileRegistry) Records(ctx context.Context, tenant string) ([]Ownership, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	b, e := os.ReadFile(r.Path)
	if errors.Is(e, os.ErrNotExist) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var records []Ownership
	if e = json.Unmarshal(b, &records); e != nil {
		return nil, errors.New("invalid ownership registry")
	}
	out := []Ownership{}
	for _, v := range records {
		if v.Tenant == tenant {
			out = append(out, v)
		}
	}
	return out, nil
}
func (r FileRegistry) Register(ctx context.Context, v Ownership) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if !portable(v.Tenant) || v.OperationID == "" || v.BackupID == "" || v.Created.IsZero() || v.Provenance == "" {
		return errors.New("incomplete ownership provenance")
	}
	dir := filepath.Dir(r.Path)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	root, e := os.OpenRoot(dir)
	if e != nil {
		return e
	}
	defer root.Close()
	name := filepath.Base(r.Path)
	lock := name + ".lock"
	if e = root.Mkdir(lock, 0700); e != nil {
		return e
	}
	defer root.Remove(lock)
	records := []Ownership{}
	b, e := root.ReadFile(name)
	if e == nil {
		if e = json.Unmarshal(b, &records); e != nil {
			return errors.New("invalid ownership registry")
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	for _, old := range records {
		if old.Tenant == v.Tenant && old.BackupID == v.BackupID {
			return errors.New("duplicate ownership record")
		}
	}
	records = append(records, v)
	b, e = json.MarshalIndent(records, "", "  ")
	if e != nil {
		return e
	}
	tmp := name + "-" + token()
	defer root.Remove(tmp)
	if e = writeSync(root, tmp, append(b, '\n')); e != nil {
		return e
	}
	if e = root.Rename(tmp, name); e != nil {
		return e
	}
	return syncDir(root, ".")
}
