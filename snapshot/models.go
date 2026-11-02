package snapshot

import "time"

type Artifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Data   []byte `json:"-"`
}
type Changes struct {
	Created   []string `json:"created"`
	Modified  []string `json:"modified"`
	Unchanged []string `json:"unchanged"`
	Deleted   []string `json:"deleted"`
}
type Manifest struct {
	Schema    int            `json:"schema"`
	Tenant    string         `json:"tenant"`
	JobID     string         `json:"job_id"`
	Completed time.Time      `json:"completed"`
	Types     []string       `json:"types"`
	Counts    map[string]int `json:"counts"`
	Artifacts []Artifact     `json:"artifacts"`
	Changes   Changes        `json:"changes"`
}
type Snapshot struct {
	Manifest  Manifest
	Artifacts []Artifact
}
