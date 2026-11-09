package exporter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Engine creates and downloads remote exports without organizing or saving files.
type Engine struct {
	// Config supplies type filters, timing and retry policy.
	Config Config
	// API supplies remote export operations and is required.
	API ExportAPI
}

// Download contains the complete remote document and the job and type selection
// needed by BuildSnapshot. Its zero value is not a valid completed export.
type Download struct {
	// JobID is the remote export job ID.
	JobID string
	// Types is the selected export scope, including types with no returned objects.
	Types []string
	// Raw is the full downloaded JSON document, including tenant and objects.
	Raw json.RawMessage
}

// ExportAndDownload discovers types, applies filters, creates one job, polls
// until COMPLETE and downloads its full JSON document. It does not write files.
// Creation is not retried: a failure may still have created a remote job.
// The returned Download retains any job ID supplied by the provider, including
// alongside an initiation error or on a later failure, for inspection.
// Context cancellation stops local work, not the remote job; provider authentication
// can have separate cancellation limits. BuildSnapshot validates the downloaded data.
func (e Engine) ExportAndDownload(ctx context.Context) (Download, error) {
	var out Download
	if err := e.Config.Validate(); err != nil {
		return out, err
	}
	if e.API == nil {
		return out, errors.New("API is required")
	}
	timeout, _ := ParseDuration(e.Config.Timeout)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	supported, err := e.API.Discover(ctx)
	if err != nil {
		return out, fmt.Errorf("cannot discover exportable types: %w", err)
	}
	known := map[string]bool{}
	for _, s := range supported {
		if !validComponent(s) {
			return out, errors.New("ISC returned an invalid exportable type name")
		}
		known[s] = true
	}
	for _, s := range append(append([]string{}, e.Config.Include...), e.Config.Exclude...) {
		if !known[s] {
			return out, fmt.Errorf("type %q is not exportable in this tenant; check include and exclude in the exporter policy", s)
		}
	}
	selected := map[string]bool{}
	if len(e.Config.Include) == 0 {
		for s := range known {
			selected[s] = true
		}
	} else {
		for _, s := range e.Config.Include {
			selected[s] = true
		}
	}
	for _, s := range e.Config.Exclude {
		delete(selected, s)
	}
	for s := range selected {
		out.Types = append(out.Types, s)
	}
	sort.Strings(out.Types)
	if len(out.Types) == 0 {
		return out, errors.New("no exportable types remain after filtering; check include and exclude in the exporter policy")
	}
	id, err := e.API.Start(ctx, out.Types)
	out.JobID = id
	if err != nil {
		return out, fmt.Errorf("cannot start export; the request was not retried, so inspect tenant export jobs before repeating: %w", err)
	}
	if id == "" {
		return out, errors.New("ISC returned no export job ID; inspect tenant export jobs before starting another export")
	}
	poll, _ := ParseDuration(e.Config.PollInterval)
	for {
		var status string
		status, err = e.API.Status(ctx, id)
		if err != nil {
			return out, fmt.Errorf("cannot check export progress: %w", err)
		}
		// SP-Config export statuses: https://developer.sailpoint.com/docs/tools/sdk/powershell/beta/models/sp-config-export-job-status/
		// The pinned Go SDK exposes Status as a string, without enum constants.
		switch status {
		case "COMPLETE":
			out.Raw, err = e.API.Download(ctx, id)
			if err != nil {
				return out, fmt.Errorf("cannot download completed export: %w", err)
			}
			return out, nil
		case "NOT_STARTED", "IN_PROGRESS":
			if err = wait(ctx, poll); err != nil {
				return out, err
			}
		case "FAILED", "CANCELLED":
			return out, fmt.Errorf("ISC export job ended with status %s; inspect the job in ISC", status)
		default:
			return out, errors.New("unrecognized export job status")
		}
	}
}

func wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
