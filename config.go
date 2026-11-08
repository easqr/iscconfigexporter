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
		if errors.Is(err, io.EOF) {
			return c, errors.New("exporter policy is empty; start with exporter.example.json")
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return c, errors.New("exporter policy JSON is incomplete; check closing quotes, brackets and braces")
		}
		var fieldType *json.UnmarshalTypeError
		if errors.As(err, &fieldType) && fieldType.Field != "" {
			return c, fmt.Errorf("policy field %q has the wrong JSON type; see docs/CONFIGURATION.md", fieldType.Field)
		}
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) {
			return c, fmt.Errorf("invalid JSON near byte %d; check commas, quotes and brackets", syntax.Offset)
		}
		return c, errors.New("expected an exporter policy JSON object with supported fields and field types; see docs/CONFIGURATION.md")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return c, errors.New("exporter policy must contain exactly one JSON document; remove trailing content")
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
		return errors.New("tenant must be 1-100 ASCII letters, digits, hyphens or underscores, start with a letter or digit, and not be a reserved device name")
	}

	if strings.TrimSpace(c.Output) == "" {
		return errors.New("output must specify a directory for exported snapshots")
	}
	for _, setting := range []struct{ name, value string }{
		{"timeout", c.Timeout}, {"poll_interval", c.PollInterval}, {"backoff", c.Backoff},
	} {
		d, e := Duration(setting.value)
		if e != nil || d <= 0 {
			return fmt.Errorf("%s must be a positive duration, such as 30s, 10m or 7d", setting.name)
		}
	}
	if c.Retries < 0 || c.Retries > 10 {
		return errors.New("retries must be between 0 and 10")
	}
	if c.LogLevel != "info" && c.LogLevel != "debug" && c.LogLevel != "warn" && c.LogLevel != "error" {
		return errors.New("log_level must be debug, info, warn or error")
	}
	seen := map[string]bool{}
	for _, s := range c.Include {
		if !validComponent(s) || seen[s] {
			return errors.New("include must contain unique type names using 1-100 ASCII letters, digits, hyphens or underscores, starting with a letter or digit")
		}
		seen[s] = true
	}
	ex := map[string]bool{}
	for _, s := range c.Exclude {
		if !validComponent(s) || seen[s] || ex[s] {
			return errors.New("exclude must contain unique valid type names and must not overlap include; see docs/CONFIGURATION.md")
		}
		ex[s] = true
	}
	b := c.Backup
	switch b.Policy {
	case "disabled":
		if b.Enabled {
			return errors.New("backup.enabled requires backup.policy to be older_than or keep_latest")
		}
	case "older_than":
		d, e := Duration(b.Age)
		if e != nil || d <= 0 {
			return errors.New("backup.age must be a positive duration for older_than, such as 7d")
		}
	case "keep_latest":
		if b.Age != "" {
			return errors.New("backup.age must be omitted when backup.policy is keep_latest")
		}
	default:
		return errors.New("backup.policy must be disabled, older_than or keep_latest")
	}
	return nil
}
