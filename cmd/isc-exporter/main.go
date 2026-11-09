package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	exporter "github.com/easqr/iscconfigexporter"
	"github.com/easqr/iscconfigexporter/internal/buildinfo"
	"github.com/easqr/iscconfigexporter/sdk"
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
		fmt.Fprintln(os.Stderr, "error:", cliErrorMessage(e))
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string, out, errOut io.Writer) error {
	return runWithAdapter(ctx, args, out, errOut, func(c exporter.Config) (*sdk.Adapter, error) { return sdk.New(c, nil) })
}
func runWithAdapter(ctx context.Context, args []string, out, errOut io.Writer, newAdapter func(exporter.Config) (*sdk.Adapter, error)) error {
	flags := flag.NewFlagSet("isc-exporter", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	version := flags.Bool("version", false, "print version, commit, modification state, Go version and platform without loading configuration")
	p := flags.String("config", "exporter.json", "exporter policy `file`; SDK connection settings use a separate config.json")
	validate := flags.Bool("validate", false, "check local policy and options without contacting ISC or checking credentials")
	preview := flags.Bool("cleanup-preview", false, "preview the enabled backup policy after exporting; still writes an export")
	retrieve := flags.Bool("retrieve-mappings", false, "save Configuration Hub mappings instead of exporting configuration")
	source := flags.String("mapping-source-org", "", "source `organization` for -retrieve-mappings; use default for unassociated mappings")
	directory := flags.String("mapping-output", "", "new archive `directory` for -retrieve-mappings; must be outside the export tree")
	job := flags.String("mapping-export-job", "", "optional export job `ID` to record with mappings; the association is not verified")
	showHelp := func() {
		fmt.Fprintln(errOut, `Export SailPoint ISC configuration to local snapshots.

Usage:
  isc-exporter [options]

Examples:
  isc-exporter -version
  isc-exporter -config exporter.json -validate
  isc-exporter -config exporter.json
  isc-exporter -config exporter.json -cleanup-preview
  isc-exporter -config exporter.json -retrieve-mappings \
    -mapping-source-org default -mapping-output ./mapping-archives/new-archive

Connection settings: SDK config.json or SAIL_* environment variables.
Backup evaluation is disabled by default; this version cannot delete backups.
Mapping retrieval requires both -mapping-source-org and -mapping-output.
Use -validate with either action to check local settings; it does not verify API access.
Exports and mapping retrieval write JSON results to standard output.
Logs and errors go to standard error; SDK token-request failures may also write to standard output.

Options (use -help or -h to show this help):`)
		flags.SetOutput(errOut)
		flags.PrintDefaults()
		flags.SetOutput(io.Discard)
	}
	flags.Usage = func() {} // Parse errors are reported once by main.
	if e := flags.Parse(args); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			showHelp()
			return nil
		}
		return fmt.Errorf("%w; run isc-exporter -help for usage", e)
	}
	if flags.NArg() != 0 {
		return errors.New("this command does not accept positional arguments; use -config to select a policy file and -help for usage")
	}
	if *version {
		info := buildinfo.Read()
		_, err := fmt.Fprintf(out, "isc-exporter %s\ncommit: %s\nmodified: %s\ngo: %s\nplatform: %s/%s\n", info.Version, info.Commit, info.Modified, info.GoVersion, info.GOOS, info.GOARCH)
		return err
	}
	f, e := os.Open(*p)
	if e != nil {
		return fmt.Errorf("cannot open exporter policy: %w; use -config to select a policy file", e)
	}
	c, e := exporter.LoadConfig(f)
	ce := f.Close()
	if e != nil {
		return fmt.Errorf("invalid exporter policy %q: %w", *p, e)
	}
	if ce != nil {
		return fmt.Errorf("cannot close exporter policy %q: %w", *p, ce)
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
			return errors.New("-retrieve-mappings and -cleanup-preview cannot be used together; choose one action")
		}
		if *source == "" {
			return errors.New("-retrieve-mappings requires -mapping-source-org; specify a source organization or default")
		}
		if *directory == "" {
			return errors.New("-retrieve-mappings requires -mapping-output; choose a new directory outside the export tree")
		}
		if e = request.Validate(); e != nil {
			return fmt.Errorf("invalid mapping options: %w", e)
		}
		if e = exporter.ValidateMappingDestination(*directory, c.Output); e != nil {
			return fmt.Errorf("invalid -mapping-output: %w", e)
		}
	} else if mappingOptions {
		return errors.New("-mapping-source-org, -mapping-output and -mapping-export-job require -retrieve-mappings")
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
		return fmt.Errorf("cannot configure the ISC connection: %w", e)
	}
	if *retrieve {
		timeout, e := exporter.ParseDuration(c.Timeout)
		if e != nil {
			return e
		}
		requestCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		snapshot, e := exporter.RetrieveMappings(requestCtx, api, request)
		if e != nil {
			return fmt.Errorf("cannot retrieve mappings: %w", e)
		}
		publication, e := (exporter.MappingFileStore{Directory: *directory, ExportRoot: c.Output}).Publish(requestCtx, snapshot)
		if e != nil {
			return fmt.Errorf("cannot save mapping archive: %w", e)
		}
		if e := json.NewEncoder(out).Encode(publication); e != nil {
			return fmt.Errorf("mapping archive was saved, but its JSON result could not be written: %w", e)
		}
		return nil
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
		return fmt.Errorf("export snapshot was saved, but its JSON result could not be written: %w", e)
	}
	if result.CleanupError != "" {
		return errors.New(result.CleanupError)
	}
	return nil
}

// Simplify cancellation only for CLI display; execution retains wrapped errors.
func cliErrorMessage(err error) string {
	message := err.Error()
	interrupted := false
	if errors.Is(err, context.Canceled) {
		message = "operation canceled"
		interrupted = true
	} else if errors.Is(err, context.DeadlineExceeded) {
		message = "operation timed out; check the connection and the timeout setting in the exporter policy"
		interrupted = true
	}
	var failure *exporter.ExportError
	if errors.As(err, &failure) && failure.JobID != "" {
		if interrupted {
			return fmt.Sprintf("%s (export job %q); the remote job may continue; inspect it in ISC before retrying", message, failure.JobID)
		}
		message += "; inspect the job in ISC before retrying"
	}
	return message
}
