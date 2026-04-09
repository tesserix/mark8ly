// services/marketplace-api/internal/stores/platform_http.go
package stores

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// HTTPClient is a real Client backed by platform-api's internal store
// lookup endpoint (GET /internal/stores/:id). It's wired in main.go when
// MARKETPLACE_PLATFORM_API_URL is set; otherwise StoreMiddleware falls
// back to the stub platform client and tests pre-seed the projection
// table directly.
type HTTPClient struct {
	baseURL string
	secret  string
	client  *http.Client
}

// NewHTTPClient constructs an HTTPClient. The secret, when non-empty, is
// sent as X-Internal-Auth on every request — defense-in-depth alongside
// Istio's network policy. httpClient may be nil; defaults to a 3-second
// timeout.
func NewHTTPClient(baseURL, secret string, httpClient *http.Client) *HTTPClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 3 * time.Second}
	}
	return &HTTPClient{
		baseURL: baseURL,
		secret:  secret,
		client:  httpClient,
	}
}

// GetStore satisfies the Client interface.
func (c *HTTPClient) GetStore(ctx context.Context, tenantID, storeID string) (*Store, error) {
	url := fmt.Sprintf("%s/internal/stores/%s", c.baseURL, storeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("stores: platform client: new req: %w", err)
	}
	if c.secret != "" {
		req.Header.Set("X-Internal-Auth", c.secret)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stores: platform client: %w", errors.Join(ErrPlatformUnavailable, err))
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// ok — continue
	case http.StatusNotFound:
		return nil, nil
	default:
		return nil, fmt.Errorf("stores: platform client: unexpected status %d: %w",
			resp.StatusCode, ErrPlatformUnavailable)
	}

	var envelope struct {
		Data Store `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("stores: platform client: decode: %w", err)
	}
	if envelope.Data.TenantID != tenantID {
		return nil, nil
	}
	return &envelope.Data, nil
}

// GetStoreBySlug calls the PUBLIC by-slug endpoint on platform-api. The
// public response intentionally omits tenant_id, so the returned Store
// has TenantID == "" — callers on the storefront path must not rely on
// TenantID from this source. Returns (nil, nil) on 404 and wraps any
// transport or 5xx error with ErrPlatformUnavailable.
func (c *HTTPClient) GetStoreBySlug(ctx context.Context, slug string) (*Store, error) {
	url := fmt.Sprintf("%s/stores/by-slug/%s", c.baseURL, slug)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("stores: platform client by-slug: new req: %w", err)
	}
	if c.secret != "" {
		req.Header.Set("X-Internal-Auth", c.secret)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stores: platform client by-slug: %w", errors.Join(ErrPlatformUnavailable, err))
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// ok — continue
	case http.StatusNotFound:
		return nil, nil
	default:
		return nil, fmt.Errorf("stores: platform client by-slug: unexpected status %d: %w",
			resp.StatusCode, ErrPlatformUnavailable)
	}

	// The public response shape deliberately has no tenant_id field.
	var envelope struct {
		Data struct {
			ID           string `json:"id"`
			Slug         string `json:"slug"`
			Name         string `json:"name"`
			CountryCode  string `json:"country_code"`
			CurrencyCode string `json:"currency_code"`
			Timezone     string `json:"timezone"`
			Status       string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("stores: platform client by-slug: decode: %w", err)
	}
	return &Store{
		ID:           envelope.Data.ID,
		TenantID:     "", // not exposed on the public endpoint
		Slug:         envelope.Data.Slug,
		Name:         envelope.Data.Name,
		CountryCode:  envelope.Data.CountryCode,
		CurrencyCode: envelope.Data.CurrencyCode,
		Timezone:     envelope.Data.Timezone,
		Status:       envelope.Data.Status,
	}, nil
}
