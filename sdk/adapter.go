// Package sdk isolates all SailPoint-generated models and authentication.
package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hashicorp/go-retryablehttp"
	sailpoint "github.com/sailpoint-oss/golang-sdk/v2"
	beta "github.com/sailpoint-oss/golang-sdk/v2/api_beta"
	"github.com/spf13/viper"
	exporter "gitlab.com/easqrtech/iam/iscconfigexporter"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Adapter delegates API construction, authentication and HTTP retries
// to the pinned SailPoint SDK. Its gate protects the SDK's mutable token caches.
type Adapter struct {
	client *sailpoint.APIClient
	gate   chan struct{}
}

// New loads SDK settings from SAIL_* or local config.json, without a home-directory
// fallback. No authentication or network request occurs during construction.
// transport is an optional API transport.
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
		return nil, errors.New("SDK BaseURL must be an HTTPS URL containing only a host and optional port; check config.json or SAIL_BASE_URL")
	}
	// Normalize a harmless trailing slash, without constructing an OAuth endpoint.
	conn.BaseURL = strings.TrimRight(conn.BaseURL, "/")
	if conn.Token == "" {
		if conn.ClientId == "" || conn.ClientSecret == "" {
			return nil, errors.New("SDK ClientId and ClientSecret are required when no Token is supplied; check config.json or SAIL_CLIENT_ID and SAIL_CLIENT_SECRET")
		}
		if !httpsEndpoint(conn.TokenURL) {
			return nil, errors.New("SDK TokenURL must use HTTPS without user information, a query string or a fragment")
		}
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
func defaultConfiguration() (*sailpoint.Configuration, error) {
	var conn sailpoint.ClientConfiguration
	if base := os.Getenv("SAIL_BASE_URL"); base != "" {
		conn = sailpoint.ClientConfiguration{
			BaseURL:      base,
			ClientId:     os.Getenv("SAIL_CLIENT_ID"),
			ClientSecret: os.Getenv("SAIL_CLIENT_SECRET"),
			NermBaseURL:  os.Getenv("SAIL_NERM_BASE_URL"),
		}
	} else {
		executable, err := os.Executable()
		if err != nil {
			return nil, configurationLoadError()
		}
		// Select exact filenames; Viper's name search also accepts other extensions.
		v := viper.New()
		for _, path := range []string{filepath.Join(filepath.Dir(executable), "config.json"), "config.json"} {
			if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
				continue
			} else if err != nil {
				return nil, configurationLoadError()
			}
			v.SetConfigFile(path)
			if err := v.ReadInConfig(); err != nil {
				return nil, configurationLoadError()
			}
			break
		}
		if v.ConfigFileUsed() == "" {
			return nil, configurationLoadError()
		}
		if err := v.Unmarshal(&conn); err != nil {
			return nil, configurationLoadError()
		}
	}
	conn.TokenURL = strings.TrimRight(conn.BaseURL, "/") + "/oauth/token"
	return sailpoint.NewConfiguration(conn), nil
}
func configurationLoadError() error {
	return errors.New("cannot load SDK connection settings; provide valid config.json beside the executable or in the working directory, or set SAIL_BASE_URL, SAIL_CLIENT_ID and SAIL_CLIENT_SECRET")
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
	guidance := "check the tenant connection settings and try again if appropriate"
	switch e.code {
	case http.StatusUnauthorized:
		guidance = "check the SDK credentials or supplied token"
	case http.StatusForbidden:
		guidance = "check the tenant role and API permissions for this operation"
	case http.StatusNotFound:
		guidance = "check the tenant API URL and whether the requested resource still exists"
	case http.StatusTooManyRequests:
		guidance = "the tenant request limit was reached; wait before retrying"
	default:
		if e.code >= 500 {
			guidance = "ISC reported a server error; check service availability before retrying"
		}
	}
	return fmt.Sprintf("ISC request failed (HTTP %d); %s", e.code, guidance)
}
func safe(ctx context.Context, r *http.Response, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		if r != nil {
			return apiError{r.StatusCode}
		}
		return errors.New("cannot connect to ISC or authenticate; check the SDK connection URL, credentials and network access")
	}
	if r == nil {
		return errors.New("ISC returned no response; check the connection and network access")
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
		return "", errors.New("ISC did not return export job details; inspect tenant export jobs before starting another export")
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
	if v == nil {
		return "", errors.New("ISC returned no export status; inspect tenant export jobs before retrying")
	}
	if v.JobId != id {
		return "", errors.New("ISC returned status for a different export job; inspect tenant export jobs before retrying")
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
		return nil, errors.New("cannot read the downloaded export; check the connection and retry the export if needed")
	}
	return b, nil
}

var _ exporter.API = (*Adapter)(nil)
