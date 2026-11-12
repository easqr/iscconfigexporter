package exporter

import (
	"context"
	"time"

	snapfmt "github.com/easqr/iscconfigexporter/snapshot"
)

// FilenameCheck reports exact ID and name compatibility for a validated download.
// Safe concerns filename components and inventory collisions on Linux, macOS and
// Windows, not filesystem support or destination-specific full path limits.
type FilenameCheck struct {
	Tenant  string                   `json:"tenant"`
	JobID   string                   `json:"job_id"`
	Objects int                      `json:"objects"`
	Modes   []snapfmt.FilenameReport `json:"modes"`
}

func snapshotObjects(s snapfmt.Snapshot) ([]snapfmt.Object, error) {
	objects := make([]snapfmt.Object, 0, len(s.Artifacts))
	for _, a := range s.Artifacts {
		v, e := snapfmt.DecodeJSON(a.Data)
		if e != nil {
			return nil, e
		}
		env := v.(map[string]any)
		self := env["self"].(map[string]any)
		objects = append(objects, snapfmt.Object{Type: self["type"].(string), ID: self["id"].(string), Path: a.Path, Envelope: env})
	}
	return objects, nil
}

// CheckFilenames validates a download and assesses both alternate naming modes.
// Compatibility issues are data in Modes; malformed downloads return an error.
// It performs no filesystem or network operations.
func CheckFilenames(tenant string, d Download) (FilenameCheck, error) {
	return CheckFilenamesWithEscaping(tenant, d, false)
}

// CheckFilenamesWithEscaping reports both alternate naming modes using the
// requested escaping policy, including proposed paths for safe objects.
func CheckFilenamesWithEscaping(tenant string, d Download, escape bool) (FilenameCheck, error) {
	return CheckFilenamesWithOptions(tenant, d, snapfmt.FilenameOptions{Escape: escape})
}

// CheckFilenamesWithOptions assesses ID and name modes with the configured policies.
func CheckFilenamesWithOptions(tenant string, d Download, opts snapfmt.FilenameOptions) (FilenameCheck, error) {
	s, e := buildHexSnapshot(tenant, d, time.Time{})
	if e != nil {
		return FilenameCheck{}, e
	}
	objects, e := snapshotObjects(s)
	if e != nil {
		return FilenameCheck{}, e
	}
	return FilenameCheck{Tenant: tenant, JobID: d.JobID, Objects: len(objects), Modes: []snapfmt.FilenameReport{
		snapfmt.CheckFilenamesWithOptions(objects, snapfmt.FilenameOptions{Mode: snapfmt.IDFilename, Escape: opts.Escape}), snapfmt.CheckFilenamesWithOptions(objects, snapfmt.FilenameOptions{Mode: snapfmt.NameFilename, Escape: opts.Escape, AppendID: opts.AppendID}),
	}}, nil
}

// CheckFilenames starts and downloads an ISC export to assess ID and name modes.
// It does not call Store or Lifecycle or publish local files. Store may be nil.
// The remote export job still exists; this is not an offline policy validation.
func (s Service) CheckFilenames(ctx context.Context) (FilenameCheck, error) {
	timeout, e := ParseDuration(s.Engine.Config.Timeout)
	if e != nil {
		return FilenameCheck{}, e
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	d, e := s.Engine.ExportAndDownload(ctx)
	if e != nil {
		return FilenameCheck{}, exportFailure(d.JobID, e)
	}
	report, e := CheckFilenamesWithOptions(s.Engine.Config.Tenant, d, snapfmt.FilenameOptions{Escape: s.Engine.Config.FilenameEscape, AppendID: s.Engine.Config.FilenameAppendID})
	if e != nil {
		return report, exportFailure(d.JobID, e)
	}
	return report, nil
}
