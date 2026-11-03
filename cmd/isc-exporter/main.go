package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	exporter "github.com/elianderson/iscutilities/configexporter"
	"github.com/elianderson/iscutilities/configexporter/sdk"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if e := run(ctx, os.Args[1:], os.Stdout, os.Stderr); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string, out, errOut io.Writer) error {
	return runWithAdapter(ctx, args, out, errOut, func(c exporter.Config) (*sdk.Adapter, error) { return sdk.New(c, nil) })
}
func runWithAdapter(ctx context.Context, args []string, out, errOut io.Writer, newAdapter func(exporter.Config) (*sdk.Adapter, error)) error {
	flags := flag.NewFlagSet("isc-exporter", flag.ContinueOnError)
	flags.SetOutput(errOut)
	p := flags.String("config", "exporter.json", "exporter policy file (SDK connection uses config.json)")
	validate := flags.Bool("validate", false, "validate local configuration without credentials or network")
	preview := flags.Bool("cleanup-preview", false, "force cleanup dry-run after successful export publication")
	retrieve := flags.Bool("retrieve-mappings", false, "retrieve Configuration Hub mappings only; no configuration export")
	source := flags.String("mapping-source-org", "", "mapping source organization, or default for unassociated mappings")
	directory := flags.String("mapping-output", "", "new mapping bundle directory, separate from configuration exports")
	job := flags.String("mapping-export-job", "", "optional operator-supplied export job correlation")
	if e := flags.Parse(args); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			return nil
		}
		return e
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	f, e := os.Open(*p)
	if e != nil {
		return e
	}
	c, e := exporter.LoadConfig(f)
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	mappingOptions := false
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "mapping-source-org", "mapping-output", "mapping-export-job":
			mappingOptions = true
		}
	})
	request := exporter.MappingRequest{SourceOrg: *source, TargetTenant: c.Tenant, ExportJobID: *job}
	if *retrieve {
		if *preview {
			return errors.New("mapping retrieval cannot be combined with cleanup preview")
		}
		if e = request.Validate(); e != nil {
			return e
		}
		if e = exporter.ValidateMappingDestination(*directory, c.Output); e != nil {
			return e
		}
	} else if mappingOptions {
		return errors.New("mapping options require -retrieve-mappings")
	}
	if *validate {
		_, e = fmt.Fprintln(out, "configuration valid")
		return e
	}
	if *preview {
		c.Backup.DryRun = true
	}
	api, e := newAdapter(c)
	if e != nil {
		return e
	}
	if *retrieve {
		timeout, e := exporter.Duration(c.Timeout)
		if e != nil {
			return e
		}
		requestCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		snapshot, e := exporter.RetrieveMappings(requestCtx, api, request)
		if e != nil {
			return e
		}
		publication, e := (exporter.MappingFileStore{Directory: *directory, ExportRoot: c.Output}).Publish(requestCtx, snapshot)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(publication)
	}
	level := slog.LevelInfo
	_ = level.UnmarshalText([]byte(c.LogLevel))
	logger := slog.New(slog.NewJSONHandler(errOut, &slog.HandlerOptions{Level: level}))
	lifecycle := exporter.BackupManager{API: api.Backups(c)}
	if c.Backup.Registry != "" {
		lifecycle.Registry = exporter.FileRegistry{Path: c.Backup.Registry}
	}
	service := exporter.Service{Engine: exporter.Engine{Config: c, API: api}, Store: exporter.FileStore{Output: c.Output}, Lifecycle: lifecycle, Logger: logger}
	result, e := service.Export(ctx)
	if e != nil {
		return e
	}
	if e = json.NewEncoder(out).Encode(result); e != nil {
		return e
	}
	if result.CleanupError != "" {
		return errors.New(result.CleanupError)
	}
	return nil
}
