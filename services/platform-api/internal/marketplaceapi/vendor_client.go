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
	baseURL            string
	internalAuthSecret string
	http               *http.Client
}

// NewVendorClient constructs a client pointed at the given base URL
// (e.g. "http://mark8ly-marketplace-api-admin.mark8ly.svc.cluster.local:8080").
// A 10-second default timeout is applied; the caller can swap the client
// by assigning to the embedded field if a different policy is needed.
func NewVendorClient(baseURL string, internalAuthSecret ...string) *VendorClient {
	secret := ""
	if len(internalAuthSecret) > 0 {
		secret = internalAuthSecret[0]
	}
	return &VendorClient{
		baseURL:            baseURL,
		internalAuthSecret: secret,
		http:               &http.Client{Timeout: 10 * time.Second},
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
	c.addInternalAuth(req)

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
	c.addInternalAuth(req)

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

func (c *VendorClient) addInternalAuth(req *http.Request) {
	if c.internalAuthSecret != "" {
		req.Header.Set("X-Internal-Auth", c.internalAuthSecret)
	}
}

// Store mirrors marketplace-api's local stores projection. Only the
// fields platform-api cares about are decoded; the wire format may carry
// more (synced_at, secret) that we ignore.
type Store struct {
	ID           string `json:"id"`
	TenantID     string `json:"tenant_id"`
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	CountryCode  string `json:"country_code"`
	CurrencyCode string `json:"currency_code"`
	Timezone     string `json:"timezone"`
	Status       string `json:"status"`
}

// EnsureSubscription gives a newly created store its subscription row, and
// with it the start of its 90-day trial.
//
// Called from onboarding.Complete AFTER EnsureSelfStore, and the order is a
// requirement rather than a preference: marketplace_api's
// store_subscriptions.store_id is a foreign key onto its stores projection,
// so the row this mirrors has to land first.
//
// Idempotent on store id. Best-effort like the two calls beside it — a
// failure is logged and does not fail onboarding — but the consequence is
// worth naming: a store with no subscription row has no trial clock at all
// and is invisible to every billing cron until something else creates one
// (mark8ly#827).
func (c *VendorClient) EnsureSubscription(ctx context.Context, in EnsureSubscription) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/internal/stores/%s/ensure-subscription", c.baseURL, in.StoreID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.addInternalAuth(req)

	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("marketplace-api ensure-subscription: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode >= 300 {
		raw, _ := io.ReadAll(res.Body)
		return fmt.Errorf("marketplace-api ensure-subscription %d: %s", res.StatusCode, string(raw))
	}
	return nil
}

// EnsureSubscription is the request body for the call of the same name.
// StoreID travels in the path and is not serialised.
type EnsureSubscription struct {
	StoreID  string `json:"-"`
	TenantID string `json:"tenant_id"`
	Email    string `json:"email,omitempty"`
	Name     string `json:"name,omitempty"`
	// Currency is the store's ISO 4217 billing currency. Sent because
	// nothing supplies it to marketplace-api afterwards, and a subscription
	// row without one cannot have its price resolved.
	Currency string `json:"currency,omitempty"`
	// PromoCode is what the merchant typed at onboarding, if anything
	// (mark8ly#620). Redeemed on the far side, immediately after the row is
	// created — the earliest moment redemption is possible at all.
	PromoCode string `json:"promo_code,omitempty"`
}

// SignupPromoOffer is what a promo code granted, or why it was refused.
// RejectReason is already the public reason; it is safe to show a merchant.
type SignupPromoOffer struct {
	TrialExtensionDays int    `json:"trial_extension_days"`
	RejectReason       string `json:"reject_reason"`
}

// ValidatePromoForSignup asks what a code would grant a merchant who is
// signing up, WITHOUT redeeming it.
//
// This is why marketplace-api's validate route is on /internal rather than
// exposed publicly: mark8ly#620 calls an open validate endpoint "an oracle for
// guessing valid codes", and routing it through platform-api — which already
// holds the shared secret — means no such endpoint has to exist.
func (c *VendorClient) ValidatePromoForSignup(ctx context.Context, code, email, currency string) (SignupPromoOffer, bool, error) {
	var out SignupPromoOffer

	body, err := json.Marshal(map[string]string{
		"code": code, "email": email, "currency": currency,
	})
	if err != nil {
		return out, false, err
	}
	url := fmt.Sprintf("%s/internal/promo/validate-for-signup", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return out, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.addInternalAuth(req)

	res, err := c.http.Do(req)
	if err != nil {
		return out, false, fmt.Errorf("marketplace-api validate-promo: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode >= 300 {
		raw, _ := io.ReadAll(res.Body)
		return out, false, fmt.Errorf("marketplace-api validate-promo %d: %s", res.StatusCode, string(raw))
	}

	var envelope struct {
		Data struct {
			Valid bool `json:"valid"`
			SignupPromoOffer
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		return out, false, fmt.Errorf("marketplace-api validate-promo: decode: %w", err)
	}
	return envelope.Data.SignupPromoOffer, envelope.Data.Valid, nil
}

// EnsureSelfStore upserts the authoritative store row from platform_api
// into marketplace_api's local stores projection. Idempotent and keyed
// on store id — re-runs are safe and preserve any server-side fields
// (e.g. storefront_customer_portal_secret) that aren't sent over.
//
// Called from onboarding.Complete right after EnsureSelfVendor. A
// failure here is logged but does NOT fail onboarding (matching the
// vendor client's best-effort policy); the merchant can retry by
// updating store settings or running the platform-api backfill CLI.
func (c *VendorClient) EnsureSelfStore(ctx context.Context, s Store) (*Store, error) {
	body, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/internal/stores/upsert", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.addInternalAuth(req)

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("marketplace-api ensure-self-store: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode >= 300 {
		raw, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("marketplace-api ensure-self-store %d: %s", res.StatusCode, string(raw))
	}

	var resp struct {
		Data Store `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("marketplace-api ensure-self-store: decode: %w", err)
	}
	return &resp.Data, nil
}

// PurgeTenant asks marketplace-api to hard-delete all data owned by the
// given tenant's stores. Called from the platform-api outbox drainer as
// part of tenant deletion; a non-2xx response is returned as an error so
// the drainer retries the purge on the next pass.
func (c *VendorClient) PurgeTenant(ctx context.Context, tenantID string, storeIDs []string) error {
	body, err := json.Marshal(map[string][]string{"store_ids": storeIDs})
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/internal/tenants/%s/purge", c.baseURL, tenantID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.addInternalAuth(req)

	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("marketplace-api purge-tenant: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode >= 300 {
		raw, _ := io.ReadAll(res.Body)
		return fmt.Errorf("marketplace-api purge-tenant %d: %s", res.StatusCode, string(raw))
	}
	return nil
}
