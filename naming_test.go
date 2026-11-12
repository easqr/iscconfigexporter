package exporter

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	snapfmt "github.com/easqr/iscconfigexporter/snapshot"
)

func namingRecord(typ, id, name string) string {
	b, _ := json.Marshal(map[string]any{"version": 1, "self": map[string]any{"type": typ, "id": id, "name": name}, "object": map[string]any{"id": id, "name": name}})
	return string(b)
}

func TestFilenameCompatibilityInventory(t *testing.T) {
	d := download(strings.Join([]string{
		namingRecord("ROLE", "a:b", "Finance/HR"),
		namingRecord("ROLE", "A", "Payroll"),
		namingRecord("ROLE", "a", "payroll"),
		namingRecord("ROLE", "unicode1", "Caf\u00e9"),
		namingRecord("ROLE", "unicode2", "Cafe\u0301"),
		namingRecord("ROLE", "CON", ""),
	}, ","))
	report, err := CheckFilenames("example", d)
	if err != nil {
		t.Fatal(err)
	}
	if report.Objects != 6 || report.Modes[0].Safe || report.Modes[1].Safe {
		t.Fatal(report)
	}
	for _, want := range []struct {
		mode     int
		id, code string
	}{
		{0, "a:b", "reserved_character"}, {0, "A", "collision"}, {0, "a", "collision"}, {0, "CON", "reserved_device"},
		{1, "a:b", "path_separator"}, {1, "A", "collision"}, {1, "a", "collision"}, {1, "CON", "missing_value"},
	} {
		found := false
		for _, issue := range report.Modes[want.mode].Issues {
			if issue.ID == want.id && issue.Code == want.code {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %+v in %+v", want, report)
		}
	}
	for _, mode := range []snapfmt.FilenameMode{snapfmt.IDFilename, snapfmt.NameFilename} {
		_, err := BuildSnapshotWithNaming("example", d, time.Now(), mode)
		var failure *snapfmt.FilenameError
		if !errors.As(err, &failure) || !strings.Contains(err.Error(), "id=") {
			t.Fatal(err)
		}
	}
	if _, err := BuildSnapshot("example", d, time.Now()); err != nil {
		t.Fatal("hex must remain available", err)
	}
}

func TestNamingPublicationReadersAndMigration(t *testing.T) {
	root := t.TempDir()
	store := FileStore{Output: root}
	d := download(namingRecord("ROLE", "abc123", "Example role"))
	var first []byte
	for _, mode := range []snapfmt.FilenameMode{snapfmt.HexFilename, snapfmt.IDFilename, snapfmt.NameFilename, snapfmt.HexFilename} {
		s, err := BuildSnapshotWithNaming("example", d, time.Now(), mode)
		if err != nil {
			t.Fatal(err)
		}
		if first == nil {
			first = s.Artifacts[0].Data
		} else if string(first) != string(s.Artifacts[0].Data) {
			t.Fatal("naming changed envelope bytes")
		}
		m, err := store.Publish(context.Background(), s)
		if err != nil {
			t.Fatal(err)
		}
		for _, readMode := range []snapfmt.Mode{snapfmt.SnapshotMode, snapfmt.TreeMode} {
			in, err := snapfmt.Read(filepath.Join(root, "example"), snapfmt.Options{Mode: readMode})
			if err != nil {
				t.Fatal(mode, readMode, err)
			}
			if len(in.Objects) != 1 || in.Objects[0].ID != "abc123" || in.Objects[0].Path != m.Artifacts[0].Path {
				t.Fatal(in)
			}
			if _, err = snapfmt.Clone(in); err != nil {
				t.Fatal(err)
			}
		}
		// Repeated export uses the same managed path and reuses unchanged content.
		m, err = store.Publish(context.Background(), s)
		if err != nil || len(m.Changes.Unchanged) != 1 {
			t.Fatal(m, err)
		}
	}
	// A name change moves the managed file, while preserving identity and unrelated files.
	s, _ := BuildSnapshotWithNaming("example", d, time.Now(), snapfmt.NameFilename)
	if _, err := store.Publish(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "example", "notes.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	s, _ = BuildSnapshotWithNaming("example", download(namingRecord("ROLE", "abc123", "Renamed role")), time.Now(), snapfmt.NameFilename)
	m, err := store.Publish(context.Background(), s)
	if err != nil || !reflect.DeepEqual(m.Changes.Deleted, []string{"ROLE/Example role.json"}) || !reflect.DeepEqual(m.Changes.Created, []string{"ROLE/Renamed role.json"}) {
		t.Fatal(m, err)
	}
	if b, err := os.ReadFile(filepath.Join(root, "example", "notes.txt")); err != nil || string(b) != "keep" {
		t.Fatal(err)
	}
}

type namingSpyStore struct{ calls int }

func (s *namingSpyStore) Publish(context.Context, snapfmt.Snapshot) (snapfmt.Manifest, error) {
	s.calls++
	return snapfmt.Manifest{}, nil
}

type namingSpyLifecycle struct{ calls int }

func (s *namingSpyLifecycle) Cleanup(context.Context, string, string, BackupConfig) (CleanupReport, error) {
	s.calls++
	return CleanupReport{}, nil
}

func TestNamingFailureAndCheckNeverPublish(t *testing.T) {
	c := testConfig()
	c.FilenameMode = snapfmt.NameFilename
	c.Backup.Enabled, c.Backup.Policy = true, "keep_latest"
	store, lifecycle := &namingSpyStore{}, &namingSpyLifecycle{}
	s := Service{Engine: Engine{Config: c, API: failingExportAPI{jobID: "job", raw: download(namingRecord("ROLE", "id", "Bad/Name")).Raw}}, Store: store, Lifecycle: lifecycle}
	_, err := s.Export(context.Background())
	var naming *snapfmt.FilenameError
	var failure *ExportError
	if !errors.As(err, &naming) || !errors.As(err, &failure) || failure.JobID != "job" {
		t.Fatal(err)
	}
	if store.calls != 0 || lifecycle.calls != 0 {
		t.Fatal("failed validation caused side effects")
	}
	report, err := s.CheckFilenames(context.Background())
	if err != nil || report.Modes[1].Safe || !report.Modes[0].Safe {
		t.Fatal(report, err)
	}
	if store.calls != 0 || lifecycle.calls != 0 {
		t.Fatal("check caused side effects")
	}
	s.Store = nil
	if _, err := s.CheckFilenames(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestFilenameConfig(t *testing.T) {
	for _, value := range []string{"hex", "id", "name"} {
		c, err := LoadConfig(strings.NewReader(`{"tenant":"example","filename_mode":"` + value + `"}`))
		if err != nil || c.FilenameMode != snapfmt.FilenameMode(value) {
			t.Fatal(c, err)
		}
	}
	if _, err := LoadConfig(strings.NewReader(`{"tenant":"example","filename_mode":"slug"}`)); err == nil {
		t.Fatal("accepted unknown mode")
	}
	legacy, err := BuildSnapshot("example", download(role), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(legacy.Manifest)
	if legacy.Manifest.Schema != 1 || strings.Contains(string(b), "filename_mode") {
		t.Fatal("legacy format changed", string(b))
	}
}

func TestEscapedInventoryAndDuplicateLifecycleNames(t *testing.T) {
	d := download(strings.Join([]string{
		namingRecord("ROLE", "source:one", "Directory/HR"),
		namingRecord("ROLE", "source%3Aone", "Directory%2FHR"),
		namingRecord("ROLE", "active1", "Active"),
		namingRecord("ROLE", "active2", "Active"),
	}, ","))
	report, err := CheckFilenamesWithEscaping("example", d, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range report.Modes {
		if !mode.Safe || !mode.Escape || len(mode.Files) != 4 {
			t.Fatal(mode)
		}
	}
	for _, mode := range []snapfmt.FilenameMode{snapfmt.IDFilename, snapfmt.NameFilename} {
		s, err := BuildSnapshotWithFilenameOptions("example", d, time.Now(), snapfmt.FilenameOptions{Mode: mode, Escape: true})
		if err != nil {
			t.Fatal(err)
		}
		if (s.Manifest.Schema != 3 && mode == snapfmt.IDFilename) || (s.Manifest.Schema != 5 && mode == snapfmt.NameFilename) || !s.Manifest.FilenameEscape {
			t.Fatal(s.Manifest)
		}
		root := t.TempDir()
		store := FileStore{Output: root}
		for i := 0; i < 2; i++ {
			if _, err = store.Publish(context.Background(), s); err != nil {
				t.Fatal(err)
			}
		}
		for _, readMode := range []snapfmt.Mode{snapfmt.SnapshotMode, snapfmt.TreeMode} {
			in, err := snapfmt.Read(filepath.Join(root, "example"), snapfmt.Options{Mode: readMode})
			if err != nil || len(in.Objects) != 4 {
				t.Fatal(in, err)
			}
			if _, err = snapfmt.Clone(in); err != nil {
				t.Fatal(err)
			}
		}
		// Reported plans must exactly match published paths, including escapes.
		paths := map[string]bool{}
		for _, a := range s.Artifacts {
			paths[a.Path] = true
		}
		for _, m := range report.Modes {
			if m.Mode == mode {
				for _, f := range m.Files {
					if !paths[f.Path] {
						t.Fatal("report did not match publication", f)
					}
				}
			}
		}
	}
	c, err := LoadConfig(strings.NewReader(`{"tenant":"example","filename_mode":"name","filename_escape":true}`))
	if err != nil || !c.FilenameEscape {
		t.Fatal(c, err)
	}
	spy, lifecycle := &namingSpyStore{}, &namingSpyLifecycle{}
	svc := Service{Engine: Engine{Config: c, API: failingExportAPI{jobID: "job", raw: d.Raw}}, Store: spy, Lifecycle: lifecycle}
	checked, err := svc.CheckFilenames(context.Background())
	if err != nil || !checked.Modes[0].Safe || !checked.Modes[1].Safe || spy.calls != 0 || lifecycle.calls != 0 {
		t.Fatal(checked, err)
	}
	if _, err = svc.Export(context.Background()); err != nil || spy.calls != 1 || lifecycle.calls != 1 {
		t.Fatal(err, spy, lifecycle)
	}
}

func TestLegacyNameGenerationMigration(t *testing.T) {
	d := download(namingRecord("ROLE", "abc", "Active"))
	s, err := BuildSnapshotWithNaming("example", d, time.Now(), snapfmt.NameFilename)
	if err != nil {
		t.Fatal(err)
	}
	// Reproduce the earlier schema 2 name-only generation on disk.
	s.Manifest.Schema = 2
	s.Manifest.Artifacts[0].Path = "ROLE/Active.json"
	root := t.TempDir()
	generation := ".isc-snapshots/example-" + strings.Repeat("a", 32)
	if err = os.MkdirAll(filepath.Join(root, generation, "ROLE"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, generation, "ROLE/Active.json"), s.Artifacts[0].Data, 0600); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(s.Manifest)
	if err = os.WriteFile(filepath.Join(root, generation, "manifest.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(generation, filepath.Join(root, "example")); err != nil {
		t.Fatal(err)
	}
	next, err := BuildSnapshotWithFilenameOptions("example", d, time.Now(), snapfmt.FilenameOptions{Mode: snapfmt.NameFilename, Escape: true, AppendID: true})
	if err != nil {
		t.Fatal(err)
	}
	m, err := (FileStore{Output: root}).Publish(context.Background(), next)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m.Changes.Deleted, []string{"ROLE/Active.json"}) || !reflect.DeepEqual(m.Changes.Created, []string{"ROLE/Active--abc.json"}) {
		t.Fatal(m.Changes)
	}
}

func lifecycleRecord(id, name string, ref any) string {
	b, _ := json.Marshal(map[string]any{"version": 1, "self": map[string]any{"type": "LIFECYCLE_STATE", "id": id, "name": name}, "object": map[string]any{"id": id, "name": name, "identityProfileRef": ref}})
	return string(b)
}

func lifecycleDownload(records ...string) Download {
	return Download{JobID: "job", Types: []string{"LIFECYCLE_STATE"}, Raw: json.RawMessage(`{"tenant":"example","objects":[` + strings.Join(records, ",") + `]}`)}
}

func TestLifecycleProfileFilenamePrefix(t *testing.T) {
	ref := func(id, name string) any { return map[string]any{"type": "IDENTITY_PROFILE", "id": id, "name": name} }
	d := lifecycleDownload(lifecycleRecord("state1", "Active", ref("p1", "Employees/US")), lifecycleRecord("state2", "Active", ref("p2", "Contractors")))
	opts := snapfmt.FilenameOptions{Mode: snapfmt.NameFilename, Escape: true}
	s, err := BuildSnapshotWithFilenameOptions("example", d, time.Now(), opts)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"LIFECYCLE_STATE/Contractors--Active.json", "LIFECYCLE_STATE/Employees%2FUS--Active.json"}
	for i, a := range s.Artifacts {
		if a.Path != want[i] {
			t.Fatal(a.Path, want[i])
		}
	}
	if s.Manifest.Schema != 5 {
		t.Fatal(s.Manifest.Schema)
	}
	report, err := CheckFilenamesWithEscaping("example", d, true)
	if err != nil || !report.Modes[1].Safe {
		t.Fatal(report, err)
	}
	for _, f := range report.Modes[1].Files {
		if f.Path != want[0] && f.Path != want[1] {
			t.Fatal(f)
		}
	}
	// Parent profiles need not be included in a filtered lifecycle-state export.
	root := t.TempDir()
	if _, err = (FileStore{Output: root}).Publish(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []snapfmt.Mode{snapfmt.SnapshotMode, snapfmt.TreeMode} {
		in, err := snapfmt.Read(filepath.Join(root, "example"), snapfmt.Options{Mode: mode})
		if err != nil || len(in.Objects) != 2 {
			t.Fatal(in, err)
		}
		if _, err = snapfmt.Clone(in); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = BuildSnapshotWithNaming("example", d, time.Now(), snapfmt.NameFilename); err == nil {
		t.Fatal("strict mode accepted slash in profile name")
	}
	// Both the profile name and state name participate in filename verification.
	files := map[string][]byte{}
	for _, a := range s.Artifacts {
		files[a.Path] = a.Data
	}
	files["manifest.json"], _ = json.Marshal(s.Manifest)
	files[want[1]] = []byte(strings.ReplaceAll(string(files[want[1]]), "Employees/US", "Employees/EU"))
	if _, err = snapfmt.FromFiles(files, snapfmt.Options{Mode: snapfmt.TreeMode}); err == nil {
		t.Fatal("accepted mismatched profile prefix")
	}
}

func TestLifecycleFilenameMissingProfile(t *testing.T) {
	for _, ref := range []any{nil, map[string]any{"type": "SOURCE", "id": "p", "name": "People"}, map[string]any{"type": "IDENTITY_PROFILE", "name": "People"}, map[string]any{"type": "IDENTITY_PROFILE", "id": "p", "name": ""}} {
		d := lifecycleDownload(lifecycleRecord("state", "Active", ref))
		r, err := CheckFilenamesWithEscaping("example", d, true)
		if err != nil || !r.Modes[0].Safe || r.Modes[1].Safe {
			t.Fatal(r, err)
		}
		found := false
		for _, issue := range r.Modes[1].Issues {
			if issue.Code == "missing_identity_profile" {
				found = true
			}
		}
		if !found {
			t.Fatal(r)
		}
		for _, mode := range []snapfmt.FilenameMode{snapfmt.HexFilename, snapfmt.IDFilename} {
			if _, err = BuildSnapshotWithNaming("example", d, time.Now(), mode); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestLifecycleLegacyNameIDMigration(t *testing.T) {
	d := lifecycleDownload(lifecycleRecord("abc", "Active", map[string]any{"type": "IDENTITY_PROFILE", "id": "p", "name": "Employees"}))
	s, err := BuildSnapshotWithNaming("example", d, time.Now(), snapfmt.NameFilename)
	if err != nil {
		t.Fatal(err)
	}
	m := s.Manifest
	m.Schema = 3
	m.Artifacts = append([]snapfmt.Artifact{}, s.Manifest.Artifacts...)
	m.Artifacts[0].Path = "LIFECYCLE_STATE/Active--abc.json"
	root := t.TempDir()
	generation := ".isc-snapshots/example-" + strings.Repeat("b", 32)
	if err = os.MkdirAll(filepath.Join(root, generation, "LIFECYCLE_STATE"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, generation, m.Artifacts[0].Path), s.Artifacts[0].Data, 0600); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(m)
	if err = os.WriteFile(filepath.Join(root, generation, "manifest.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(generation, filepath.Join(root, "example")); err != nil {
		t.Fatal(err)
	}
	if _, err = snapfmt.Read(filepath.Join(root, "example"), snapfmt.Options{Mode: snapfmt.SnapshotMode}); err != nil {
		t.Fatal(err)
	}
	published, err := (FileStore{Output: root}).Publish(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(published.Changes.Deleted, []string{"LIFECYCLE_STATE/Active--abc.json"}) || !reflect.DeepEqual(published.Changes.Created, []string{"LIFECYCLE_STATE/Employees--Active.json"}) {
		t.Fatal(published.Changes)
	}
}

func TestConditionalFilenameIDs(t *testing.T) {
	records := []string{namingRecord("ROLE", "one", "Shared"), namingRecord("ROLE", "two", "shared"), namingRecord("ROLE", "three", "Shared--one"), namingRecord("ROLE", "four", "Unique")}
	d := download(strings.Join(records, ","))
	opts := snapfmt.FilenameOptions{Mode: snapfmt.NameFilename}
	s, err := BuildSnapshotWithFilenameOptions("example", d, time.Now(), opts)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ROLE/Shared--one--three.json", "ROLE/Shared--one.json", "ROLE/Unique.json", "ROLE/shared--two.json"}
	for i, a := range s.Artifacts {
		if a.Path != want[i] {
			t.Fatal(a.Path, want[i])
		}
	}
	// Planning must be independent of export record order.
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}
	other, err := BuildSnapshotWithFilenameOptions("example", download(strings.Join(records, ",")), s.Manifest.Completed, opts)
	if err != nil || !reflect.DeepEqual(s, other) {
		t.Fatal("order changed plan", err)
	}
	root := t.TempDir()
	if _, err = (FileStore{Output: root}).Publish(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	if _, err = snapfmt.Read(filepath.Join(root, "example"), snapfmt.Options{Mode: snapfmt.SnapshotMode}); err != nil {
		t.Fatal(err)
	}
	opts.AppendID = true
	forced, err := BuildSnapshotWithFilenameOptions("example", d, time.Now(), opts)
	if err != nil || !forced.Manifest.FilenameAppendID {
		t.Fatal(err)
	}
	for _, a := range forced.Artifacts {
		if a.Path == "ROLE/Unique.json" {
			t.Fatal("forced mode omitted ID")
		}
	}
	if _, err = (FileStore{Output: root}).Publish(context.Background(), forced); err != nil {
		t.Fatal(err)
	}
	if _, err = snapfmt.Read(filepath.Join(root, "example"), snapfmt.Options{Mode: snapfmt.SnapshotMode}); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig(strings.NewReader(`{"tenant":"example","filename_mode":"name","filename_append_id":true}`))
	if err != nil || !c.FilenameAppendID || DefaultConfig().FilenameAppendID {
		t.Fatal(c, err)
	}
	report, err := (Service{Engine: Engine{Config: c, API: failingExportAPI{jobID: "job", raw: d.Raw}}}).CheckFilenames(context.Background())
	if err != nil || !report.Modes[1].AppendID {
		t.Fatal(report, err)
	}
}

func TestConditionalIDMembershipChanges(t *testing.T) {
	store := FileStore{Output: t.TempDir()}
	one := namingRecord("ROLE", "one", "Shared")
	two := namingRecord("ROLE", "two", "Shared")
	for _, records := range []string{one, one + "," + two, one} {
		s, err := BuildSnapshotWithNaming("example", download(records), time.Now(), snapfmt.NameFilename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = store.Publish(context.Background(), s); err != nil {
			t.Fatal(err)
		}
		if _, err = snapfmt.Read(filepath.Join(store.Output, "example"), snapfmt.Options{Mode: snapfmt.SnapshotMode}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(store.Output, "example", "ROLE/Shared.json")); err != nil {
		t.Fatal(err)
	}
	// A copied working tree can retain the historical suffix after deleting a peer.
	s, err := BuildSnapshotWithNaming("example", download(one+","+two), time.Now(), snapfmt.NameFilename)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	files["manifest.json"], _ = json.Marshal(s.Manifest)
	files[s.Artifacts[0].Path] = s.Artifacts[0].Data
	if _, err = snapfmt.FromFiles(files, snapfmt.Options{Mode: snapfmt.TreeMode}); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotVerifiesConditionalSuffixPolicy(t *testing.T) {
	s, err := BuildSnapshotWithNaming("example", download(namingRecord("ROLE", "one", "Unique")), time.Now(), snapfmt.NameFilename)
	if err != nil {
		t.Fatal(err)
	}
	s.Manifest.Artifacts[0].Path = "ROLE/Unique--one.json"
	files := map[string][]byte{"ROLE/Unique--one.json": s.Artifacts[0].Data}
	files["manifest.json"], _ = json.Marshal(s.Manifest)
	if _, err = snapfmt.FromFiles(files, snapfmt.Options{Mode: snapfmt.SnapshotMode}); err == nil {
		t.Fatal("accepted unnecessary suffix in conditional snapshot")
	}
	// The same suffix is canonical when the manifest explicitly forces IDs.
	s.Manifest.FilenameAppendID = true
	files["manifest.json"], _ = json.Marshal(s.Manifest)
	if _, err = snapfmt.FromFiles(files, snapfmt.Options{Mode: snapfmt.SnapshotMode}); err != nil {
		t.Fatal(err)
	}
}
