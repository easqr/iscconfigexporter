package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestReadModuleIdentity(t *testing.T) {
	const commit = "0123456789abcdef0123456789abcdef01234567"
	for _, tc := range []struct {
		name, mainPath, mainVersion, release, dependency string
		replaced                                         bool
		version, revision, modified, consumer            string
	}{
		{"release", modulePath, "(devel)", "v0.7.0", "", false, "v0.7.0", commit, "true", "0.7.0+dirty"},
		{"development", modulePath, "(devel)", "", "", false, "development", commit, "true", "development+0123456789ab.dirty"},
		{"module version", modulePath, "v0.7.0", "", "", false, "v0.7.0", commit, "true", "0.7.0+dirty"},
		{"host dependency", "example.com/host", "v9.0.0", "v9.0.0", "v0.7.0", false, "v0.7.0", "unknown", "unknown", "0.7.0"},
		{"local replacement", "example.com/host", "v9.0.0", "v9.0.0", "v0.7.0", true, "development", "unknown", "unknown", "development"},
		{"missing dependency", "example.com/host", "v9.0.0", "v9.0.0", "", false, "development", "unknown", "unknown", "development"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bi := &debug.BuildInfo{GoVersion: "go1.27.3", Main: debug.Module{Path: tc.mainPath, Version: tc.mainVersion}, Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: commit}, {Key: "vcs.modified", Value: "true"}}}
			if tc.dependency != "" {
				dep := &debug.Module{Path: modulePath, Version: tc.dependency}
				if tc.replaced {
					dep.Replace = &debug.Module{Path: "../configexporter"}
				}
				bi.Deps = []*debug.Module{{Path: "example.com/unrelated", Version: "v8.0.0"}, dep}
			}
			got := read(bi, tc.release)
			if got.Version != tc.version || got.Commit != tc.revision || got.Modified != tc.modified || got.ConsumerVersion() != tc.consumer || got.GoVersion != "go1.27.3" {
				t.Fatalf("unexpected identity: %+v, consumer %s", got, got.ConsumerVersion())
			}
		})
	}
}

func TestUnknownAndCleanMetadata(t *testing.T) {
	for _, bi := range []*debug.BuildInfo{nil, {Main: debug.Module{Path: modulePath}}} {
		got := read(bi, "")
		if got.Version != "development" || got.Commit != "unknown" || got.Modified != "unknown" || got.GoVersion == "" || got.GOOS == "" || got.GOARCH == "" {
			t.Fatalf("missing fallback: %+v", got)
		}
	}
	got := read(&debug.BuildInfo{Main: debug.Module{Path: modulePath}, Settings: []debug.BuildSetting{{Key: "vcs.modified", Value: "false"}}}, "v0.7.0")
	if got.Modified != "false" || got.ConsumerVersion() != "0.7.0" {
		t.Fatalf("clean release: %+v", got)
	}
	if got := (Info{Version: "v0.7.0+dirty", Modified: "true"}).ConsumerVersion(); got != "0.7.0+dirty" {
		t.Fatal("duplicated dirty marker", got)
	}
}
