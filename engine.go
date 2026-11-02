package exporter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Engine orchestrates reads and a single non-retried export initiation.
type Engine struct {
	Config Config
	API    API
}
type Download struct {
	JobID string
	Types []string
	Raw   json.RawMessage
}

func (e Engine) Fetch(ctx context.Context) (Download, error) {
	var out Download
	if err := e.Config.Validate(); err != nil {
		return out, err
	}
	if e.API == nil {
		return out, errors.New("API is required")
	}
	timeout, _ := Duration(e.Config.Timeout)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var supported []string
	if err := e.read(ctx, func() error { var err error; supported, err = e.API.Discover(ctx); return err }); err != nil {
		return out, fmt.Errorf("discover: %w", err)
	}
	known := map[string]bool{}
	for _, s := range supported {
		if !validComponent(s) {
			return out, errors.New("invalid discovered type")
		}
		known[s] = true
	}
	for _, s := range append(append([]string{}, e.Config.Include...), e.Config.Exclude...) {
		if !known[s] {
			return out, fmt.Errorf("unsupported type %q", s)
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
		return out, errors.New("no exportable types selected")
	}
	id, err := e.API.Start(ctx, out.Types)
	if err != nil {
		return out, fmt.Errorf("start export (not retried): %w", err)
	}
	if id == "" {
		return out, errors.New("empty export job ID")
	}
	out.JobID = id
	poll, _ := Duration(e.Config.PollInterval)
	for {
		var status string
		err = e.read(ctx, func() error { var err error; status, err = e.API.Status(ctx, id); return err })
		if err != nil {
			return out, fmt.Errorf("poll export: %w", err)
		}
		switch status {
		case "COMPLETE":
			err = e.read(ctx, func() error { var err error; out.Raw, err = e.API.Download(ctx, id); return err })
			if err != nil {
				return out, fmt.Errorf("download export: %w", err)
			}
			return out, nil
		case "NOT_STARTED", "IN_PROGRESS", "RUNNING", "PENDING":
			if err = wait(ctx, poll); err != nil {
				return out, err
			}
		case "FAILED", "CANCELLED", "FAILED_EXTERNAL_COMMUNICATION":
			return out, fmt.Errorf("export job terminated: %s", status)
		default:
			return out, errors.New("unrecognized export job status")
		}
	}
}

type transient interface{ Temporary() bool }

func (e Engine) read(ctx context.Context, fn func() error) error {
	d, _ := Duration(e.Config.Backoff)
	for i := 0; ; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := fn()
		if err == nil {
			return nil
		}
		var tr transient
		if i >= e.Config.Retries || !errors.As(err, &tr) || !tr.Temporary() {
			return err
		}
		if err = wait(ctx, d); err != nil {
			return err
		}
		if d < 30*time.Second {
			d *= 2
			if d > 30*time.Second {
				d = 30 * time.Second
			}
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
