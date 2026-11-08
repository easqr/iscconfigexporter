package exporter

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

type fakeAPI struct {
	statuses []string
	starts   int
	err      error
	raw      json.RawMessage
	types    []string
}

func (f *fakeAPI) Discover(context.Context) ([]string, error) { return []string{"ROLE", "SOURCE"}, nil }
func (f *fakeAPI) Start(_ context.Context, types []string) (string, error) {
	f.starts++
	f.types = types
	return "job", f.err
}
func (f *fakeAPI) Status(context.Context, string) (string, error) {
	if len(f.statuses) == 0 {
		return "COMPLETE", nil
	}
	s := f.statuses[0]
	if len(f.statuses) > 1 {
		f.statuses = f.statuses[1:]
	}
	return s, nil
}
func (f *fakeAPI) Download(context.Context, string) (json.RawMessage, error) { return f.raw, nil }
func TestExportAndDownload(t *testing.T) {
	for _, s := range []string{"COMPLETE", "FAILED", "CANCELLED", "FAILED_EXTERNAL_COMMUNICATION", "RUNNING", "PENDING", "BOGUS"} {
		t.Run(s, func(t *testing.T) {
			f := &fakeAPI{statuses: []string{s}}
			c := testConfig()
			c.Include = []string{"ROLE"}
			out, e := (Engine{c, f}).ExportAndDownload(context.Background())
			if (s == "RUNNING" || s == "PENDING" || s == "FAILED_EXTERNAL_COMMUNICATION") && (e == nil || e.Error() != "unrecognized export job status") {
				t.Fatalf("unexpected status accepted: %v", e)
			}
			if (e == nil) != (s == "COMPLETE") {
				t.Fatal(e)
			}
			if f.starts != 1 || len(f.types) != 1 || out.JobID != "job" {
				t.Fatal(f, out)
			}
		})
	}
	f := &fakeAPI{err: errors.New("ambiguous start")}
	_, e := (Engine{testConfig(), f}).ExportAndDownload(context.Background())
	if e == nil || f.starts != 1 {
		t.Fatal(e, f.starts)
	}
}
func TestExportAndDownloadTimeoutCancel(t *testing.T) {
	c := testConfig()
	c.Timeout = "1ms"
	f := &fakeAPI{statuses: []string{"NOT_STARTED"}}
	_, e := (Engine{c, f}).ExportAndDownload(context.Background())
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e = (Engine{testConfig(), f}).ExportAndDownload(ctx)
	if !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}

func TestFilters(t *testing.T) {
	for _, tc := range []struct {
		include, exclude []string
		want             []string
		bad              bool
	}{{nil, []string{"SOURCE"}, []string{"ROLE"}, false}, {[]string{"SOURCE"}, nil, []string{"SOURCE"}, false}, {[]string{"UNKNOWN"}, nil, nil, true}, {nil, []string{"ROLE", "SOURCE"}, nil, true}} {
		c := testConfig()
		c.Include = tc.include
		c.Exclude = tc.exclude
		f := &fakeAPI{}
		_, e := (Engine{c, f}).ExportAndDownload(context.Background())
		if (e != nil) != tc.bad {
			t.Fatal(tc, e)
		}
		if tc.bad {
			if f.starts != 0 {
				t.Fatal("started invalid selection")
			}
		} else if !reflect.DeepEqual(f.types, tc.want) {
			t.Fatal(f.types, tc.want)
		}
	}
}

func TestExportAndDownloadPollingStatuses(t *testing.T) {
	c := testConfig()
	c.PollInterval = "1ns"
	f := &fakeAPI{statuses: []string{"NOT_STARTED", "IN_PROGRESS", "COMPLETE"}, raw: json.RawMessage(`{}`)}
	if _, err := (Engine{c, f}).ExportAndDownload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.statuses) != 1 || f.statuses[0] != "COMPLETE" || f.starts != 1 {
		t.Fatal("polling did not reach completion after a single initiation")
	}
}
