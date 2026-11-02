package exporter

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Config holds export policy only. Connection and authentication use native SDK configuration.
type Config struct {
	Tenant       string       `json:"tenant"`
	Output       string       `json:"output"`
	Include      []string     `json:"include"`
	Exclude      []string     `json:"exclude"`
	Timeout      string       `json:"timeout"`
	PollInterval string       `json:"poll_interval"`
	Retries      int          `json:"retries"`
	Backoff      string       `json:"backoff"`
	LogLevel     string       `json:"log_level"`
	Backup       BackupConfig `json:"backup"`
}
type BackupConfig struct {
	Enabled  bool   `json:"enabled"`
	Policy   string `json:"policy"`
	Age      string `json:"age"`
	DryRun   bool   `json:"dry_run"`
	Registry string `json:"registry"`
}

func DefaultConfig() Config {
	return Config{Output: "exports", Timeout: "10m", PollInterval: "5s", Retries: 3, Backoff: "1s", LogLevel: "info", Backup: BackupConfig{Policy: "disabled"}}
}
func LoadConfig(r io.Reader) (Config, error) {
	c := DefaultConfig()
	d := json.NewDecoder(r)
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, errors.New("invalid configuration JSON")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return c, errors.New("configuration must contain one JSON document")
	}
	return c, c.Validate()
}

var component = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,99}$`)

func validComponent(s string) bool { return component.MatchString(s) }
func Duration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		n, e := strconv.ParseInt(strings.TrimSuffix(s, "d"), 10, 32)
		if e != nil || n <= 0 || n > 106751 {
			return 0, errors.New("invalid days")
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
func (c Config) Validate() error {
	if !portable(c.Tenant) {
		return errors.New("tenant must be a portable identifier")
	}

	if strings.TrimSpace(c.Output) == "" {
		return errors.New("output is required")
	}
	for _, s := range []string{c.Timeout, c.PollInterval, c.Backoff} {
		d, e := Duration(s)
		if e != nil || d <= 0 {
			return errors.New("timeouts, polling and backoff must be positive durations")
		}
	}
	if c.Retries < 0 || c.Retries > 10 {
		return errors.New("retries must be between 0 and 10")
	}
	if c.LogLevel != "info" && c.LogLevel != "debug" && c.LogLevel != "warn" && c.LogLevel != "error" {
		return errors.New("invalid log_level")
	}
	seen := map[string]bool{}
	for _, s := range c.Include {
		if !validComponent(s) || seen[s] {
			return errors.New("invalid or duplicate include type")
		}
		seen[s] = true
	}
	ex := map[string]bool{}
	for _, s := range c.Exclude {
		if !validComponent(s) || seen[s] || ex[s] {
			return errors.New("invalid, duplicate or contradictory exclude type")
		}
		ex[s] = true
	}
	b := c.Backup
	switch b.Policy {
	case "disabled":
		if b.Enabled {
			return errors.New("enabled cleanup requires retention policy")
		}
	case "older_than":
		d, e := Duration(b.Age)
		if e != nil || d <= 0 {
			return errors.New("cleanup age must be positive")
		}
	case "keep_latest":
		if b.Age != "" {
			return errors.New("keep_latest cannot specify age")
		}
	default:
		return fmt.Errorf("invalid retention policy")
	}
	return nil
}
