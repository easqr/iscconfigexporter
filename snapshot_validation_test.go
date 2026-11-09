package exporter

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	snapfmt "github.com/easqr/iscconfigexporter/snapshot"
)

func TestEnvelopeValidationAcrossPublicationAndReading(t *testing.T) {
	for _, tc := range []struct {
		name, record string
	}{
		{"future version", strings.Replace(role, `"version":1`, `"version":2`, 1)},
		{"zero version", strings.Replace(role, `"version":1`, `"version":0`, 1)},
		{"decimal version", strings.Replace(role, `"version":1`, `"version":1.0`, 1)},
		{"conflicting ID", strings.Replace(role, `"object":{`, `"object":{"id":"other",`, 1)},
		{"null ID", strings.Replace(role, `"object":{`, `"object":{"id":null,`, 1)},
		{"numeric ID", strings.Replace(role, `"object":{`, `"object":{"id":1,`, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := BuildSnapshot("example", download(tc.record), time.Now()); err == nil {
				t.Fatal("builder accepted an invalid envelope")
			}
			store := FileStore{Output: t.TempDir()}
			valid := snapshot(t, role)
			if _, err := store.Publish(context.Background(), valid); err != nil {
				t.Fatal(err)
			}
			before, err := CurrentPath(store.Output, "example")
			if err != nil {
				t.Fatal(err)
			}
			manifestBefore, err := os.ReadFile(filepath.Join(before, "manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			// Supply otherwise consistent metadata so publication must reject the
			// envelope itself, rather than a stale checksum or mismatched manifest.
			invalid := snapshot(t, role)
			invalid.Artifacts[0].Data = []byte(tc.record)
			invalid.Artifacts[0].SHA256 = snapfmt.Checksum(invalid.Artifacts[0].Data)
			invalid.Manifest.Artifacts = append([]snapfmt.Artifact(nil), invalid.Artifacts...)
			manifest, err := json.Marshal(invalid.Manifest)
			if err != nil {
				t.Fatal(err)
			}
			files := map[string][]byte{"manifest.json": manifest, invalid.Artifacts[0].Path: invalid.Artifacts[0].Data}
			if _, err := snapfmt.FromFiles(files, snapfmt.Options{Mode: snapfmt.SnapshotMode}); err == nil {
				t.Fatal("reader accepted an invalid envelope")
			}
			if _, err := store.Publish(context.Background(), invalid); err == nil {
				t.Fatal("store published an invalid envelope")
			}
			after, err := CurrentPath(store.Output, "example")
			if err != nil || after != before {
				t.Fatalf("rejected publication changed the current generation: %v", err)
			}
			manifestAfter, err := os.ReadFile(filepath.Join(after, "manifest.json"))
			if err != nil || string(manifestBefore) != string(manifestAfter) {
				t.Fatal("rejected publication changed the prior manifest", err)
			}
			if _, err := snapfmt.Read(after, snapfmt.Options{Mode: snapfmt.SnapshotMode}); err != nil {
				t.Fatal("prior snapshot is no longer readable", err)
			}
		})
	}
}

func TestPublishedSnapshotsRoundTrip(t *testing.T) {
	for _, record := range []string{role, strings.Replace(role, `"object":{`, `"object":{"id":"a:b",`, 1)} {
		t.Run(record, func(t *testing.T) {
			store := FileStore{Output: t.TempDir()}
			built := snapshot(t, record)
			if _, err := store.Publish(context.Background(), built); err != nil {
				t.Fatal(err)
			}
			in, err := snapfmt.Read(filepath.Join(store.Output, "example"), snapfmt.Options{Mode: snapfmt.SnapshotMode})
			if err != nil || len(in.Objects) != 1 {
				t.Fatalf("published snapshot is unreadable: %v", err)
			}
			want, err := snapfmt.DecodeJSON([]byte(record))
			if err != nil || !reflect.DeepEqual(in.Objects[0].Envelope, want) {
				t.Fatal("round trip changed fields, array order or numeric precision", err)
			}
		})
	}
}
