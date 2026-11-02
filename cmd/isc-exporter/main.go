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
	flags := flag.NewFlagSet("isc-exporter", flag.ContinueOnError)
	flags.SetOutput(errOut)
	p := flags.String("config", "exporter.json", "exporter policy file (SDK connection uses config.json)")
	validate := flags.Bool("validate", false, "validate local configuration without credentials or network")
	preview := flags.Bool("cleanup-preview", false, "force cleanup dry-run after successful export publication")
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
	if *validate {
		_, e = fmt.Fprintln(out, "configuration valid")
		return e
	}
	if *preview {
		c.Backup.DryRun = true
	}
	api, e := sdk.New(c, nil)
	if e != nil {
		return e
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
