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

	snapfmt "github.com/easqr/iscconfigexporter/snapshot"
)

// Config specifies export policy, without SDK connection settings or credentials.
// Its zero value is not valid; start with [DefaultConfig] and set Tenant. Treat
// configuration and filter slices as read-only while an export is running.
type Config struct {
	// Tenant must match the downloaded tenant. Use 1-100 ASCII letters, digits,
	// hyphens or underscores, starting with a letter or digit, and exclude reserved
	// device names. It selects local output naming, not the SDK connection URL.
	Tenant string `json:"tenant"`
	// Output is the private local directory for snapshots; the default is exports.
	Output string `json:"output"`
	// FilenameMode selects hex (default), id or name for object filenames.
	FilenameMode snapfmt.FilenameMode `json:"filename_mode"`
	// FilenameEscape percent-encodes unsafe filename characters in ID/name modes.
	// It defaults to false and has no effect in hex mode.
	FilenameEscape bool `json:"filename_escape"`
	// FilenameAppendID forces the ID suffix on all name-mode files; default false
	// adds IDs only to filenames that would otherwise collide.
	FilenameAppendID bool `json:"filename_append_id"`
	// Include selects exact exportable type names. Empty includes all discovered types.
	Include []string `json:"include"`
	// Exclude removes exact type names. It cannot overlap Include or contain duplicates.
	Exclude []string `json:"exclude"`
	// Timeout bounds the workflow, including backup evaluation; the default is 10m.
	// SDK token acquisition has separate timeout and cancellation limitations.
	Timeout string `json:"timeout"`
	// PollInterval is the delay between status checks; the default is 5s.
	PollInterval string `json:"poll_interval"`
	// Retries is the number of additional transient GET attempts, from 0 to 10;
	// the default is 3. Export initiation is not retried.
	Retries int `json:"retries"`
	// Backoff is the initial read-retry delay; the default is 1s. Delays are capped
	// at 30s except for server Retry-After values, subject to the request context.
	Backoff string `json:"backoff"`
	// LogLevel is debug, info, warn or error; the default is info.
	LogLevel string `json:"log_level"`
	// Backup controls optional post-publication backup evaluation.
	Backup BackupConfig `json:"backup"`
}

// BackupConfig controls backup evaluation and custom-provider retention behavior.
// The supplied SDK integration cannot delete backups, regardless of these settings.
type BackupConfig struct {
	// Enabled turns on evaluation after export publication; the default is false.
	Enabled bool `json:"enabled"`
	// Policy is disabled (the default), older_than or keep_latest. Enabled requires
	// one of the two retention policies.
	Policy string `json:"policy"`
	// Age is a positive [ParseDuration] value required by older_than and forbidden
	// by keep_latest. The age boundary is strict: records at the cutoff are retained.
	Age string `json:"age"`
	// DryRun reports eligible deletions without requesting them. The default is false;
	// only a verified custom provider can delete even when DryRun is false.
	DryRun bool `json:"dry_run"`
	// Registry is an optional ownership registry file path, not a Registry object.
	// The default is empty; local records alone do not establish deletion eligibility.
	Registry string `json:"registry"`
}

// DefaultConfig returns policy defaults with backup evaluation disabled. Tenant
// is empty and must be supplied before validation or export.
func DefaultConfig() Config {
	return Config{FilenameMode: snapfmt.HexFilename, Output: "exports", Timeout: "10m", PollInterval: "5s", Retries: 3, Backoff: "1s", LogLevel: "info", Backup: BackupConfig{Policy: "disabled"}}
}

// LoadConfig reads one JSON policy, applies defaults and validates local settings.
// It rejects unknown fields, wrong field types and trailing content, and neither
// loads SDK credentials nor contacts ISC. The caller retains ownership of r.
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

// ParseDuration parses Go duration syntax plus positive whole days such as 7d.
// A day is exactly 24 hours. Standard Go syntax can parse zero or negative values;
// [Config.Validate] separately requires positive policy durations.
func ParseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		n, e := strconv.ParseInt(strings.TrimSuffix(s, "d"), 10, 32)
		if e != nil || n <= 0 || n > 106751 {
			return 0, errors.New("invalid days")
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

// Validate checks policy values and filter syntax without credentials or network
// access. It does not check whether the tenant supports the selected types or
// whether the output filesystem meets [FileStore] requirements.
func (c Config) Validate() error {
	if !portable(c.Tenant) {
		return errors.New("tenant must be 1-100 ASCII letters, digits, hyphens or underscores, start with a letter or digit, and not be a reserved device name")
	}

	if !c.FilenameMode.Valid() {
		return errors.New("filename_mode must be hex, id or name")
	}
	if strings.TrimSpace(c.Output) == "" {
		return errors.New("output must specify a directory for exported snapshots")
	}
	for _, setting := range []struct{ name, value string }{
		{"timeout", c.Timeout}, {"poll_interval", c.PollInterval}, {"backoff", c.Backoff},
	} {
		d, e := ParseDuration(setting.value)
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
		d, e := ParseDuration(b.Age)
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
