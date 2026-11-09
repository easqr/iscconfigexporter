package snapshot_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/easqr/iscconfigexporter/snapshot"
)

// This example writes a small empty snapshot to demonstrate reading a saved export.
// For an existing export, start with snapshot.Read and the current tenant path.
func ExampleRead() {
	root, err := os.MkdirTemp("", "isc-snapshot-example-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(root)
	manifest, err := json.Marshal(snapshot.Manifest{
		Schema: 1, Tenant: "example", JobID: "example-job",
		Completed: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Types:     []string{"ROLE"}, Counts: map[string]int{"ROLE": 0},
	})
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), manifest, 0600); err != nil {
		panic(err)
	}

	input, err := snapshot.Read(root, snapshot.Options{Mode: snapshot.SnapshotMode})
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s: %d objects\n", input.Manifest.Tenant, len(input.Objects))
	// Output: example: 0 objects
}

// A manifest-free working tree requires explicit selected types.
func ExampleFromFiles() {
	files := map[string][]byte{
		"ROLE/id-616263313233.json": []byte(`{"version":1,"self":{"type":"ROLE","id":"abc123"},"object":{"name":"Example role"}}`),
	}
	input, err := snapshot.FromFiles(files, snapshot.Options{
		Mode: snapshot.TreeMode, Types: []string{"ROLE"}, Label: "Working copy",
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(input.Label, input.Objects[0].Type, input.Objects[0].ID)
	// Output: Working copy ROLE abc123
}
