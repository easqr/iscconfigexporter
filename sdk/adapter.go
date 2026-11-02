// Package sdk isolates all SailPoint-generated models and authentication.
package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	exporter "github.com/elianderson/iscutilities/configexporter"
	"github.com/hashicorp/go-retryablehttp"
	beta "github.com/sailpoint-oss/golang-sdk/v2/api_beta"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"io"
	"net/http"
	"strings"
	"time"
)

type Adapter struct {
	client *beta.APIClient
	auth   clientcredentials.Config
	http   *http.Client
}

// New creates an SDK adapter. No authentication or network request occurs here.
// transport may be nil for the standard transport, or a test transport.
func New(c exporter.Config, creds exporter.Credentials, transport http.RoundTripper) (*Adapter, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if creds.ClientID == "" || creds.ClientSecret == "" {
		return nil, errors.New("ISC_CLIENT_ID and ISC_CLIENT_SECRET are required")
	}
	h := &http.Client{Transport: transport, Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r := retryablehttp.NewClient()
	r.RetryMax = 0
	r.Logger = nil
	r.HTTPClient = h
	base := strings.TrimRight(c.BaseURL, "/")
	cfg := beta.NewConfiguration("", "", base+"/beta", base+"/oauth/token", "", "isc-config-exporter", false)
	cfg.HTTPClient = r
	return &Adapter{client: beta.NewAPIClient(cfg), http: h, auth: clientcredentials.Config{ClientID: creds.ClientID, ClientSecret: creds.ClientSecret, TokenURL: base + "/oauth/token"}}, nil
}
func (a *Adapter) context(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, a.http)
	return context.WithValue(ctx, beta.ContextOAuth2, a.auth.TokenSource(ctx))
}

type apiError struct{ code int }

func (e apiError) Error() string {
	return fmt.Sprintf("ISC API request failed (HTTP %d); response details withheld", e.code)
}
func (e apiError) Temporary() bool { return e.code == 429 || e.code == 408 || e.code >= 500 }
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
	v, r, e := a.client.SPConfigAPI.ListSpConfigObjects(a.context(ctx)).Execute()
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
	v, r, e := a.client.SPConfigAPI.ExportSpConfig(a.context(ctx)).ExportPayload(beta.ExportPayload{IncludeTypes: types}).Execute()
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
	v, r, e := a.client.SPConfigAPI.GetSpConfigExportStatus(a.context(ctx), id).Execute()
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
	_, r, e := a.client.SPConfigAPI.GetSpConfigExport(a.context(ctx), id).Execute()
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
