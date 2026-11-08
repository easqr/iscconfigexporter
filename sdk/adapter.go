// Package sdk isolates all SailPoint-generated models and authentication.
package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	exporter "gitlab.com/easqrtech/iam/iscconfigexporter"
	"github.com/hashicorp/go-retryablehttp"
	sailpoint "github.com/sailpoint-oss/golang-sdk/v2"
	beta "github.com/sailpoint-oss/golang-sdk/v2/api_beta"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Adapter delegates configuration, API construction, authentication and HTTP retries
// to the pinned SailPoint SDK. Its gate protects the SDK's mutable token caches.
type Adapter struct {
	client *sailpoint.APIClient
	gate   chan struct{}
}

var defaultConfigMu sync.Mutex // The SDK default loader uses global Viper state.

// New uses the SDK's native default configuration lookup. No authentication or
// network request occurs during construction. transport is an optional API transport.
func New(c exporter.Config, transport http.RoundTripper) (*Adapter, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	cfg, err := defaultConfiguration()
	if err != nil {
		return nil, err
	}
	return NewWithConfiguration(c, cfg, transport)
}

// NewWithConfiguration accepts native SDK configuration for embedded consumers.
// The caller retains ownership: neither cfg nor its HTTP client is mutated.
func NewWithConfiguration(c exporter.Config, cfg *sailpoint.Configuration, transport http.RoundTripper) (*Adapter, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, errors.New("SDK configuration is required")
	}
	copyCfg := *cfg
	conn := cfg.ClientConfiguration
	if !httpsOrigin(conn.BaseURL) {
		return nil, errors.New("SDK BaseURL must be an HTTPS origin")
	}
	// Normalize a harmless trailing slash, without constructing an OAuth endpoint.
	conn.BaseURL = strings.TrimRight(conn.BaseURL, "/")
	if conn.Token == "" && (conn.ClientId == "" || conn.ClientSecret == "" || !httpsEndpoint(conn.TokenURL)) {
		return nil, errors.New("SDK configuration requires a token or client credentials and an HTTPS TokenURL")
	}
	copyCfg.ClientConfiguration = conn
	copyCfg.Debug = false
	copyCfg.ConsumerIdentifier = "isc-config-exporter"
	copyCfg.ConsumerVersion = "development"
	copyCfg.DefaultHeader = make(map[string]string, len(cfg.DefaultHeader))
	for k, v := range cfg.DefaultHeader {
		copyCfg.DefaultHeader[k] = v
	}
	r := retryablehttp.NewClient()

	h := &http.Client{Timeout: 60 * time.Second}
	if cfg.HTTPClient != nil && cfg.HTTPClient.HTTPClient != nil {
		*h = *cfg.HTTPClient.HTTPClient
		if h.Timeout == 0 {
			h.Timeout = 60 * time.Second
		}
	}
	if transport != nil {
		h.Transport = transport
	}
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	r.HTTPClient = h
	r.Logger = nil
	r.RetryMax = c.Retries
	r.RetryWaitMin, _ = exporter.Duration(c.Backoff)
	r.RetryWaitMax = 30 * time.Second
	if r.RetryWaitMin > r.RetryWaitMax {
		r.RetryWaitMin = r.RetryWaitMax
	}
	r.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		// A missing response is ambiguous; never retry initiation or auth failures.
		if resp == nil || resp.Request == nil || resp.Request.Method != http.MethodGet {
			return false, nil
		}
		return resp.StatusCode == 408 || resp.StatusCode == 429 || resp.StatusCode >= 500, nil
	}
	r.Backoff = retryablehttp.DefaultBackoff
	r.ErrorHandler = retryablehttp.PassthroughErrorHandler
	copyCfg.HTTPClient = r
	return &Adapter{client: sailpoint.NewAPIClient(&copyCfg), gate: make(chan struct{}, 1)}, nil
}
func defaultConfiguration() (cfg *sailpoint.Configuration, err error) {
	defaultConfigMu.Lock()
	defer defaultConfigMu.Unlock()
	// SDK discovery panics on missing/malformed configuration. Do not expose its
	// panic text, which may contain configuration contents.
	defer func() {
		if recover() != nil {
			cfg = nil
			err = errors.New("SailPoint SDK configuration could not be loaded; configure the SDK using config.json or SAIL_* settings")
		}
	}()
	return sailpoint.NewDefaultConfiguration(), nil
}
func httpsOrigin(s string) bool {
	u, e := url.Parse(s)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Path == "" || u.Path == "/")
}
func httpsEndpoint(s string) bool {
	u, e := url.Parse(s)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}
func (a *Adapter) acquire(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	select {
	case a.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (a *Adapter) release() { <-a.gate }

type apiError struct{ code int }

func (e apiError) Error() string {
	return fmt.Sprintf("ISC API request failed (HTTP %d); response details withheld", e.code)
}
func safe(ctx context.Context, r *http.Response, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		if r != nil {
			return apiError{r.StatusCode}
		}
		return errors.New("ISC transport or authentication failed; details withheld")
	}
	if r == nil {
		return errors.New("missing ISC response")
	}
	return nil
}
func closeResponse(r *http.Response) {
	if r != nil && r.Body != nil {
		_ = r.Body.Close()
	}
}
func (a *Adapter) Discover(ctx context.Context) ([]string, error) {
	if err := a.acquire(ctx); err != nil {
		return nil, err
	}
	defer a.release()
	v, r, e := a.client.Beta.SPConfigAPI.ListSpConfigObjects(ctx).Execute()
	defer closeResponse(r)
	if e = safe(ctx, r, e); e != nil {
		return nil, e
	}
	var out []string
	for _, t := range v {
		if t.GetExportable() {
			out = append(out, t.GetObjectType())
		}
	}
	return out, nil
}
func (a *Adapter) Start(ctx context.Context, types []string) (string, error) {
	if err := a.acquire(ctx); err != nil {
		return "", err
	}
	defer a.release()
	v, r, e := a.client.Beta.SPConfigAPI.ExportSpConfig(ctx).ExportPayload(beta.ExportPayload{IncludeTypes: types}).Execute()
	defer closeResponse(r)
	if e = safe(ctx, r, e); e != nil {
		return "", e
	}
	if v == nil {
		return "", errors.New("empty export response")
	}
	return v.JobId, nil
}
func (a *Adapter) Status(ctx context.Context, id string) (string, error) {
	if err := a.acquire(ctx); err != nil {
		return "", err
	}
	defer a.release()
	v, r, e := a.client.Beta.SPConfigAPI.GetSpConfigExportStatus(ctx, id).Execute()
	defer closeResponse(r)
	if e = safe(ctx, r, e); e != nil {
		return "", e
	}
	if v == nil || v.JobId != id {
		return "", errors.New("export status identity mismatch")
	}
	return v.Status, nil
}
func (a *Adapter) Download(ctx context.Context, id string) (json.RawMessage, error) {
	if err := a.acquire(ctx); err != nil {
		return nil, err
	}
	defer a.release()
	_, r, e := a.client.Beta.SPConfigAPI.GetSpConfigExport(ctx, id).Execute()
	defer closeResponse(r)
	if e = safe(ctx, r, e); e != nil {
		return nil, e
	}
	b, e := io.ReadAll(r.Body)
	if e != nil {
		return nil, errors.New("cannot read export response")
	}
	return b, nil
}

var _ exporter.API = (*Adapter)(nil)
