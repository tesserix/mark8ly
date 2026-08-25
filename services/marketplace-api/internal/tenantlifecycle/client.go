// Package tenantlifecycle is a marketplace-api client for platform-api's
// internal tenant suspend/unsuspend endpoints (#287).
//
// This is the platform-admin surface's first WRITE client. Every existing
// client (tenantdirectory, onboardingfunnel, estatecounts) is read-only.
// The status mapping below is the part that matters: an error must never
// be conflated with an empty or zero result. A caller that received
// {StoresAffected: 0, Changed: false} from a failed call would report
// "nothing to do" for a request that may well have suspended a tenant.
package tenantlifecycle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ErrUnavailable signals platform-api could not be reached, or answered 5xx.
var ErrUnavailable = errors.New("tenantlifecycle: platform-api unavailable")

// ErrNotFound signals platform-api returned 404 for a tenant id.
var ErrNotFound = errors.New("tenantlifecycle: tenant not found")

// ErrConflict signals platform-api returned 409 for an invalid status
// transition (e.g. suspending an archived tenant).
var ErrConflict = errors.New("tenantlifecycle: invalid status transition")

// ErrUpstreamRouteMissing signals a 404 that did NOT come from the handler:
// its body is not this surface's JSON error envelope, so nothing upstream
// looked up a tenant and decided it was absent. Gin answers a bare
// `404 page not found` for an unmatched route, and during a rolling deploy
// where marketplace-api ships ahead of platform-api that is exactly what a
// teardown gets.
//
// It WRAPS ErrUnavailable so callers keep answering 503 for it: "we do not
// know" is the honest answer. Mapping it to ErrNotFound would surface as
// `404 tenant_not_found`, which this API's contract defines as "including
// already purged" — telling an operator working a GDPR erasure that the
// tenant is already destroyed, when it is alive.
var ErrUpstreamRouteMissing = fmt.Errorf("%w: teardown route not mounted upstream", ErrUnavailable)

// maxBody caps what we will read from platform-api.
const maxBody = 4 << 20

// Result is the outcome of a suspend/unsuspend call.
type Result struct {
	TenantID       string `json:"tenant_id"`
	Status         string `json:"status"`
	StoresAffected int    `json:"stores_affected"`
	Changed        bool   `json:"changed"`
}

// Client calls platform-api's internal tenant lifecycle endpoints.
type Client struct {
	baseURL string
	secret  string
	http    *http.Client
}

// NewClient constructs a Client. httpClient may be nil (defaults to a
// 5-second timeout). The secret is sent as X-Internal-Auth when non-empty.
func NewClient(baseURL, secret string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	return &Client{baseURL: baseURL, secret: secret, http: httpClient}
}

// post issues a POST to path and decodes the response body into out.
//
// Status mapping: 200 decodes; 404 -> ErrNotFound; 409 -> ErrConflict;
// everything else, including every 5xx and any transport error,
// -> ErrUnavailable. A 200 whose body is truncated or unparseable is an
// error, never a zero result.
func (c *Client) post(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("tenantlifecycle: build request: %w", err)
	}
	if c.secret != "" {
		req.Header.Set("X-Internal-Auth", c.secret)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
		// fall through to decode below
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode == http.StatusConflict:
		return ErrConflict
	default:
		return fmt.Errorf("%w: upstream %d", ErrUnavailable, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("%w: read body: %v", ErrUnavailable, err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("tenantlifecycle: decode: %w", err)
	}
	return nil
}

// Suspend calls POST /internal/tenants/:id/suspend.
func (c *Client) Suspend(ctx context.Context, tenantID string) (*Result, error) {
	return c.lifecycle(ctx, tenantID, "suspend")
}

// Unsuspend calls POST /internal/tenants/:id/unsuspend.
func (c *Client) Unsuspend(ctx context.Context, tenantID string) (*Result, error) {
	return c.lifecycle(ctx, tenantID, "unsuspend")
}

func (c *Client) lifecycle(ctx context.Context, tenantID, action string) (*Result, error) {
	var envelope struct {
		Data Result `json:"data"`
	}
	path := "/internal/tenants/" + url.PathEscape(tenantID) + "/" + action
	if err := c.post(ctx, path, &envelope); err != nil {
		return nil, err
	}
	return &envelope.Data, nil
}

// ConfirmationMismatchError signals platform-api refused a teardown
// because the supplied store-slug set did not match the tenant's actual
// set. Expected carries the actual set, so the console can refresh without
// a second round trip.
type ConfirmationMismatchError struct {
	Expected []string
}

func (e *ConfirmationMismatchError) Error() string {
	return fmt.Sprintf("tenantlifecycle: store slug confirmation mismatch; expected %v", e.Expected)
}

// TeardownResult is the outcome of an operator-initiated tenant teardown.
// StoreIDs is what marketplace-api scopes its own purge by.
type TeardownResult struct {
	TenantID   string   `json:"tenant_id"`
	TenantName string   `json:"tenant_name"`
	StoreIDs   []string `json:"store_ids"`
	StoreSlugs []string `json:"store_slugs"`
}

// Teardown calls POST /internal/tenants/:id/teardown (#288). IRREVERSIBLE.
//
// It does not reuse `post`: that helper sends no body and discards
// non-200 bodies, and this call needs both — the confirmation set going
// up, and the 409's `expected` set coming back.
//
// storeSlugs is marshalled as an ARRAY even when empty. A nil slice
// marshals to `null`, which upstream reads as an ABSENT confirmation and
// refuses with 400 — and "I assert this tenant has no stores" is a
// legitimate request that must reach the check.
func (c *Client) Teardown(ctx context.Context, tenantID string, storeSlugs []string) (*TeardownResult, error) {
	if storeSlugs == nil {
		storeSlugs = []string{}
	}
	payload, err := json.Marshal(struct {
		StoreSlugs []string `json:"store_slugs"`
	}{StoreSlugs: storeSlugs})
	if err != nil {
		return nil, fmt.Errorf("tenantlifecycle: encode teardown body: %w", err)
	}

	path := "/internal/tenants/" + url.PathEscape(tenantID) + "/teardown"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("tenantlifecycle: build teardown request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		req.Header.Set("X-Internal-Auth", c.secret)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody))

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through
	case http.StatusNotFound:
		// A 404 means "no such tenant" ONLY if the handler produced it.
		// See ErrUpstreamRouteMissing for why the two must not collapse.
		if readErr != nil || !isErrorEnvelope(body) {
			return nil, ErrUpstreamRouteMissing
		}
		return nil, ErrNotFound
	case http.StatusConflict:
		var mismatch struct {
			Expected []string `json:"expected"`
		}
		if readErr == nil {
			_ = json.Unmarshal(body, &mismatch)
		}
		if mismatch.Expected == nil {
			mismatch.Expected = []string{}
		}
		return nil, &ConfirmationMismatchError{Expected: mismatch.Expected}
	case http.StatusBadRequest:
		return nil, fmt.Errorf("tenantlifecycle: teardown rejected: %s", string(body))
	default:
		return nil, fmt.Errorf("%w: upstream %d", ErrUnavailable, resp.StatusCode)
	}

	if readErr != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrUnavailable, readErr)
	}
	var envelope struct {
		Data TeardownResult `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("tenantlifecycle: decode teardown: %w", err)
	}
	if envelope.Data.StoreIDs == nil {
		envelope.Data.StoreIDs = []string{}
	}
	if envelope.Data.StoreSlugs == nil {
		envelope.Data.StoreSlugs = []string{}
	}
	return &envelope.Data, nil
}

// isErrorEnvelope reports whether body is this surface's JSON error
// envelope — `{"error": "...", ...}` with a non-empty code, as produced by
// platform-api's respondError. A bare `404 page not found`, an HTML error
// page from an ingress, an empty body, and `{}` all fail it.
//
// The `error` field must be non-empty, not merely present: a JSON document
// with no such key unmarshals silently into the zero value, so testing only
// for a decode error would accept any well-formed JSON at all.
func isErrorEnvelope(body []byte) bool {
	var env struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return false
	}
	return env.Error != ""
}
