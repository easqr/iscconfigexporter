package exporter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	snapfmt "gitlab.com/easqrtech/iam/iscconfigexporter/snapshot"
)

type failingExportAPI struct {
	jobID                                         string
	discoverErr, startErr, statusErr, downloadErr error
	raw                                           json.RawMessage
}

func (f failingExportAPI) Discover(context.Context) ([]string, error) {
	return []string{"ROLE", "SOURCE"}, f.discoverErr
}
func (f failingExportAPI) Start(context.Context, []string) (string, error) {
	return f.jobID, f.startErr
}
func (f failingExportAPI) Status(context.Context, string) (string, error) {
	return "COMPLETE", f.statusErr
}
func (f failingExportAPI) Download(context.Context, string) (json.RawMessage, error) {
	return f.raw, f.downloadErr
}

type failingStore struct {
	manifest snapfmt.Manifest
	err      error
}

func (s failingStore) Publish(context.Context, snapfmt.Snapshot) (snapfmt.Manifest, error) {
	return s.manifest, s.err
}

func TestServiceFailureJobContext(t *testing.T) {
	sentinel := errors.New("provider unavailable")
	underlying := &os.PathError{Op: "read", Path: "private-output", Err: sentinel}
	visible := snapfmt.Manifest{Schema: 1, Tenant: "example", JobID: "known-job"}
	for _, stage := range []string{"discovery", "start without ID", "start with ID", "poll", "download", "prepare", "publish", "visible publication", "canceled", "deadline"} {
		t.Run(stage, func(t *testing.T) {
			api := failingExportAPI{jobID: "known-job", raw: download(role).Raw}
			store := failingStore{}
			wantID := "known-job"
			var wantManifest snapfmt.Manifest
			var cause error = underlying
			switch stage {
			case "discovery":
				api.discoverErr = underlying
				wantID = ""
			case "start without ID":
				api.jobID, api.startErr = "", underlying
				wantID = ""
			case "start with ID":
				api.startErr = underlying
			case "poll":
				api.statusErr = underlying
			case "download":
				api.downloadErr = underlying
			case "prepare":
				api.raw = []byte(`{"tenant":"example","objects":null}`)
				cause = nil
			case "publish":
				store.err = underlying
			case "visible publication":
				store.err, store.manifest = underlying, visible
				wantManifest = visible
			case "canceled":
				cause = context.Canceled
				api.statusErr = fmt.Errorf("poll stopped: %w", cause)
			case "deadline":
				cause = context.DeadlineExceeded
				api.downloadErr = fmt.Errorf("download stopped: %w", cause)
			}
			result, err := (Service{Engine: Engine{Config: testConfig(), API: api}, Store: store}).Export(context.Background())
			if err == nil {
				t.Fatal("expected workflow failure")
			}
			var failure *ExportError
			if errors.As(err, &failure) != (wantID != "") {
				t.Fatalf("unexpected job context: %v", err)
			}
			if failure != nil && (failure.JobID != wantID || !strings.Contains(failure.Error(), wantID)) {
				t.Fatalf("lost job ID: %v", failure)
			}
			if cause != nil && !errors.Is(err, cause) {
				t.Fatalf("lost underlying error: %v", err)
			}
			if cause == underlying {
				var pathErr *os.PathError
				if !errors.As(err, &pathErr) || pathErr != underlying || !errors.Is(err, sentinel) {
					t.Fatal("lost nested error identity", err)
				}
			}
			if !reflect.DeepEqual(result.Manifest, wantManifest) {
				t.Fatal("invented or discarded publication metadata", result.Manifest)
			}
		})
	}
}
