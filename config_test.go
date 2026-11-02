package exporter

import (
	"strings"
	"testing"
)

func testConfig() Config {
	c := DefaultConfig()
	c.Tenant = "example"
	return c
}
func TestConfig(t *testing.T) {
	if e := testConfig().Validate(); e != nil {
		t.Fatal(e)
	}
	for _, mut := range []func(*Config){func(c *Config) { c.Tenant = "../x" }, func(c *Config) { c.Timeout = "0s" }, func(c *Config) { c.Backup.Enabled = true }, func(c *Config) { c.Include = []string{"ROLE"}; c.Exclude = []string{"ROLE"} }, func(c *Config) { c.Backup = BackupConfig{Policy: "older_than", Age: "-1d"} }} {
		c := testConfig()
		mut(&c)
		if c.Validate() == nil {
			t.Fatal("accepted invalid config")
		}
	}
	if _, e := LoadConfig(strings.NewReader(`{"secret":"x"}`)); e == nil {
		t.Fatal("unknown field")
	}
	if d, e := Duration("7d"); e != nil || d.Hours() != 168 {
		t.Fatal(d, e)
	}
}
