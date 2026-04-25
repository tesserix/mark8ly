// Package marketplaceapi is the platform-api-side HTTP client for
// marketplace-api's internal endpoints. Only the endpoints platform-api
// actually needs are implemented — this is not a full SDK.
package marketplaceapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Vendor mirrors the shape returned by marketplace-api's vendor
// endpoints. Only fields platform-api needs are decoded — the wire
// format may carry more (created_at/updated_at) that we ignore.
type Vendor struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Status   string `json:"status"`
	IsSelf   bool   `json:"is_self"`
}

// VendorClient is a thin HTTP client for marketplace-api's
// /internal/tenants/:tenantID/ensure-self-vendor endpoint.
type VendorClient struct {
	baseURL string
	http    *http.Client
}

// NewVendorClient constructs a client pointed at the given base URL
// (e.g. "http://mark8ly-marketplace-api-admin.mark8ly.svc.cluster.local:8080").
// A 10-second default timeout is applied; the caller can swap the client
// by assigning to the embedded field if a different policy is needed.
func NewVendorClient(baseURL string) *VendorClient {
	return &VendorClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// EnsureSelfVendor calls the idempotent endpoint. Safe to call any
// number of times per tenant; repeated calls return the existing vendor
// unchanged.
func (c *VendorClient) EnsureSelfVendor(ctx context.Context, tenantID, name, slug string) (*Vendor, error) {
	body, err := json.Marshal(map[string]string{"name": name, "slug": slug})
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/internal/tenants/%s/ensure-self-vendor", c.baseURL, tenantID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("marketplace-api ensure-self-vendor: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode >= 300 {
		raw, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("marketplace-api ensure-self-vendor %d: %s", res.StatusCode, string(raw))
	}

	var resp struct {
		Data Vendor `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("marketplace-api ensure-self-vendor: decode: %w", err)
	}
	return &resp.Data, nil
}

// UpdateSelfVendor overwrites the name and slug of the tenant's
// self-vendor. Returns an error if the tenant has no self-vendor
// (404 from marketplace-api).
func (c *VendorClient) UpdateSelfVendor(ctx context.Context, tenantID, name, slug string) (*Vendor, error) {
	body, err := json.Marshal(map[string]string{"name": name, "slug": slug})
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/internal/tenants/%s/self-vendor", c.baseURL, tenantID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("marketplace-api update-self-vendor: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode >= 300 {
		raw, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("marketplace-api update-self-vendor %d: %s", res.StatusCode, string(raw))
	}

	var resp struct {
		Data Vendor `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("marketplace-api update-self-vendor: decode: %w", err)
	}
	return &resp.Data, nil
}
