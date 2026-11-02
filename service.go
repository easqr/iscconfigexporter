package exporter

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// Service composes export, organization, publication and post-publication cleanup.
type Service struct {
	Engine    Engine
	Store     Store
	Lifecycle Lifecycle
	Logger    *slog.Logger
}

func (s Service) Export(ctx context.Context) (Result, error) {
	var out Result
	if s.Store == nil {
		return out, errors.New("artifact store is required")
	}
	timeout, e := Duration(s.Engine.Config.Timeout)
	if e != nil {
		return out, e
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	d, e := s.Engine.Fetch(ctx)
	if e != nil {
		return out, e
	}
	snap, e := Organize(s.Engine.Config.Tenant, d, time.Now())
	if e != nil {
		return out, fmt.Errorf("organize: %w", e)
	}
	out.Manifest, e = s.Store.Publish(ctx, snap)
	if e != nil {
		return out, fmt.Errorf("publish: %w", e)
	}
	if s.Logger != nil {
		s.Logger.InfoContext(ctx, "export published", "objects", len(snap.Artifacts))
	}
	if s.Lifecycle != nil {
		out.Cleanup, e = s.Lifecycle.Cleanup(ctx, s.Engine.Config.Tenant, d.JobID, s.Engine.Config.Backup)
		if e != nil {
			out.CleanupError = "backup cleanup failed; export remains published"
		}
	} else if s.Engine.Config.Backup.Enabled {
		out.Cleanup.Limitation = "backup cleanup adapter unavailable; no deletion performed"
	}
	return out, nil
}

var _ Exporter = Service{}
