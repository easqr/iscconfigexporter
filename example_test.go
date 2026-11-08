package exporter_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	exporter "gitlab.com/easqrtech/iam/iscconfigexporter"
)

// This example uses a local provider so it can run without tenant credentials.
// For ISC, construct the provider with sdk.New using SDK connection settings.
func ExampleService_Export() {
	output, err := os.MkdirTemp("", "isc-export-example-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(output)

	policy := exporter.DefaultConfig()
	policy.Tenant = "example"
	policy.Output = output
	service := exporter.Service{
		Engine: exporter.Engine{Config: policy, API: exampleExportAPI{}},
		Store:  exporter.FileStore{Output: output},
	}
	result, err := service.Export(context.Background())
	if err != nil {
		panic(err)
	}
	if result.CleanupError != "" {
		// Backup evaluation can fail independently of a saved export.
		fmt.Println(result.CleanupError)
	}
	fmt.Println("Roles:", result.Manifest.Counts["ROLE"])
	// Output: Roles: 1
}

// This example retrieves and saves mappings using a local response provider.
// A real SDK adapter implements MappingAPI with the same interface.
func ExampleRetrieveMappings() {
	root, err := os.MkdirTemp("", "isc-mapping-example-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(root)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	mappings, err := exporter.RetrieveMappings(ctx, exampleMappingAPI{}, exporter.MappingRequest{
		SourceOrg: "default", TargetTenant: "example",
	})
	if err != nil {
		panic(err)
	}
	archive, err := (exporter.MappingFileStore{
		Directory:  filepath.Join(root, "mappings"),
		ExportRoot: filepath.Join(root, "exports"),
	}).Publish(ctx, mappings)
	if err != nil {
		panic(err)
	}
	fmt.Println("Mappings:", archive.Provenance.Count)
	// Output: Mappings: 0
}

func ExampleParseDuration() {
	duration, err := exporter.ParseDuration("7d")
	if err != nil {
		panic(err)
	}
	fmt.Println(duration)
	// Output: 168h0m0s
}

type exampleExportAPI struct{}

func (exampleExportAPI) Discover(context.Context) ([]string, error) {
	return []string{"ROLE"}, nil
}
func (exampleExportAPI) Start(context.Context, []string) (string, error) {
	return "example-job", nil
}
func (exampleExportAPI) Status(context.Context, string) (string, error) {
	return "COMPLETE", nil
}
func (exampleExportAPI) Download(context.Context, string) (json.RawMessage, error) {
	return json.RawMessage(`{"tenant":"example","objects":[{"version":1,"self":{"type":"ROLE","id":"abc123"},"object":{"name":"Example role"}}]}`), nil
}

type exampleMappingAPI struct{}

func (exampleMappingAPI) GetMappings(context.Context, string) (exporter.MappingDownload, error) {
	return exporter.MappingDownload{
		Data: json.RawMessage(`[]`), APIOrigin: "https://example.invalid",
	}, nil
}
