package exporter

import "fmt"

// ExportError is a Service.Export failure for which the remote export job ID is
// known. It does not imply that the job failed remotely or that no local snapshot
// was published. Inspect the returned Result and the job in ISC before retrying.
// Use errors.As to retrieve the ID and errors.Is/errors.As to inspect Err.
type ExportError struct {
	// JobID is the ID returned by the export provider, not a backup ID.
	JobID string
	// Err is the underlying workflow error. Custom providers and stores must
	// keep credentials and payloads out of errors; this wrapper preserves their text.
	Err error
}

// Error describes the failed export, quoting JobID to escape control characters.
func (e *ExportError) Error() string {
	return fmt.Sprintf("export job %q: %v", e.JobID, e.Err)
}

// Unwrap preserves access to the underlying error, including cancellation and deadlines.
func (e *ExportError) Unwrap() error { return e.Err }

func exportFailure(jobID string, err error) error {
	if jobID == "" {
		return err
	}
	return &ExportError{JobID: jobID, Err: err}
}
