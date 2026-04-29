// Package cfclient is a small Cloudflare API v4 client scoped to the
// operations the custom-domain "Cloudflare (auto)" flow needs:
//
//   1. Find the zone for a merchant's domain (Zone:Read).
//   2. Upsert a CNAME pointing the domain at our edge (DNS:Edit).
//   3. Verify the record still exists / read its current target.
//   4. Delete the record on takedown.
//
// Tokens come from the merchant — we do not hold a platform-wide CF
// token. The token is supplied per call so that all operations the
// platform performs against the merchant's Cloudflare account are
// authorised by their own scoped token. Tokens are read out of GCP
// Secret Manager by the caller; this package never logs them.
package cfclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// cfIDPattern is the shape Cloudflare uses for zone / record / account
// IDs (32-char lowercase hex). Validating before we paste IDs into a
// URL path stops a corrupt or malicious response from rewriting our
// request to a different endpoint via an embedded slash.
var cfIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

func validCFID(s string) bool { return cfIDPattern.MatchString(s) }

const (
	defaultBaseURL    = "https://api.cloudflare.com/client/v4"
	defaultHTTPTimout = 15 * time.Second
)

// Client talks to api.cloudflare.com. Construct via New; the zero value
// is not usable.
//
// CnameTarget is the platform-controlled hostname the merchant's CNAME
// is pointed at — typically "edge.mark8ly.com". It's a Client field
// rather than a per-call argument so the domain.CloudflareClient
// interface stays stable across implementations.
type Client struct {
	httpC       *http.Client
	baseURL     string
	cnameTarget string
}

// Option mutates a Client at construction.
type Option func(*Client)

// WithBaseURL overrides the API root. Used by tests pointing at httptest.
func WithBaseURL(u string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(u, "/")
	}
}

// WithHTTPClient overrides the underlying *http.Client. Useful for
// timeout customisation or test transports.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.httpC = h
		}
	}
}

// New constructs a client with sane production defaults: a 15s
// timeout, the public CF API base URL, and no shared state across
// callers. A single instance is safe for concurrent use.
//
// cnameTarget is required — every CNAME we create points at this
// hostname so TLS terminates at our own ingress.
func New(cnameTarget string, opts ...Option) *Client {
	c := &Client{
		httpC:       &http.Client{Timeout: defaultHTTPTimout},
		baseURL:     defaultBaseURL,
		cnameTarget: cnameTarget,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// AddDomain creates (or updates if one already exists) a DNS-only CNAME
// at `domain` pointing to the platform's configured edge hostname.
// Returns the zone ID and the DNS record ID so the caller can persist
// them for later verify/teardown calls.
//
// The CNAME is created with proxied=false because TLS termination
// happens at our own ingress (cert-manager + Istio Gateway), not at the
// Cloudflare edge — Cloudflare's edge can't present a cert for an
// arbitrary merchant FQDN.
//
// Signature matches domain.CloudflareClient so this client can be
// passed in as the production implementation without an adapter.
func (c *Client) AddDomain(ctx context.Context, domain, apiToken string) (zoneID, recordID string, err error) {
	if c.cnameTarget == "" {
		return "", "", errors.New("cfclient: cnameTarget not configured")
	}
	apexZone, err := apexOf(domain)
	if err != nil {
		return "", "", err
	}

	zoneID, err = c.findZoneID(ctx, apexZone, apiToken)
	if err != nil {
		return "", "", err
	}

	recordID, err = c.upsertCNAME(ctx, zoneID, domain, c.cnameTarget, apiToken)
	if err != nil {
		return "", "", err
	}
	return zoneID, recordID, nil
}

// VerifyDomain returns (recordExists, sslActive, err). For the
// auto-CNAME flow we manage SSL ourselves via cert-manager — sslActive
// here only reflects whether Cloudflare reports the zone as active for
// DNS, not whether HTTPS is live for the merchant. The cert-manager
// flow updates ssl_status separately on RefreshCertStatus.
func (c *Client) VerifyDomain(ctx context.Context, zoneID, domain, apiToken string) (verified, sslActive bool, err error) {
	rec, err := c.findCNAME(ctx, zoneID, domain, apiToken)
	if err != nil {
		return false, false, err
	}
	if rec == nil {
		return false, false, nil
	}
	// We don't track CF-edge SSL — return false for sslActive so the
	// caller's cert-manager poller is the sole source of truth.
	return true, false, nil
}

// RemoveDomain deletes the DNS record for the merchant's domain.
// NotFound is treated as success so the call is idempotent (e.g. if
// the merchant already removed it manually).
func (c *Client) RemoveDomain(ctx context.Context, zoneID, recordID, apiToken string) error {
	if zoneID == "" || recordID == "" {
		return nil
	}
	if !validCFID(zoneID) || !validCFID(recordID) {
		return errors.New("cfclient: refusing to call CF with malformed zone/record ID")
	}
	endpoint := fmt.Sprintf("%s/zones/%s/dns_records/%s", c.baseURL, zoneID, recordID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	c.authorize(req, apiToken)
	resp, err := c.httpC.Do(req)
	if err != nil {
		return fmt.Errorf("cfclient: delete record: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode >= 400 {
		return parseAPIError(resp)
	}
	return nil
}

// ─── Internal helpers ─────────────────────────────────────────────────

// findZoneID resolves a zone name (the apex, e.g. example.com) to its
// CF zone ID. Returns a 4xx-style error when the token can't see the
// zone (typical user-facing case: token scoped to the wrong zone).
func (c *Client) findZoneID(ctx context.Context, apexZone, apiToken string) (string, error) {
	q := url.Values{}
	q.Set("name", apexZone)
	endpoint := c.baseURL + "/zones?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	c.authorize(req, apiToken)
	resp, err := c.httpC.Do(req)
	if err != nil {
		return "", fmt.Errorf("cfclient: list zones: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", apperrors.ValidationFailed(
			"cf_api_token",
			"Cloudflare rejected the API token. Check that the token has Zone:Read + DNS:Edit and is scoped to this domain's zone.",
		)
	}
	if resp.StatusCode >= 400 {
		return "", parseAPIError(resp)
	}

	var body struct {
		Success bool `json:"success"`
		Result  []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("cfclient: decode zones: %w", err)
	}
	if !body.Success || len(body.Result) == 0 {
		return "", apperrors.ValidationFailed(
			"domain",
			fmt.Sprintf("No Cloudflare zone found for %s. Make sure the domain is added to your Cloudflare account and the API token includes its zone.", apexZone),
		)
	}
	id := body.Result[0].ID
	if !validCFID(id) {
		return "", errors.New("cfclient: cloudflare returned an unexpected zone id format")
	}
	return id, nil
}

// upsertCNAME creates a DNS-only CNAME, or updates it in place if one
// already exists for the same name. Returns the record ID.
func (c *Client) upsertCNAME(ctx context.Context, zoneID, name, target, apiToken string) (string, error) {
	existing, err := c.findCNAME(ctx, zoneID, name, apiToken)
	if err != nil {
		return "", err
	}

	payload := dnsRecordRequest{
		Type:    "CNAME",
		Name:    name,
		Content: target,
		TTL:     1, // 1 = "automatic" in CF
		Proxied: false,
		Comment: "mark8ly automated CNAME — TLS terminates at edge.mark8ly.com",
	}

	if existing != nil {
		// Update in place. We always force content to our edge so a
		// merchant who manually pointed it elsewhere is corrected.
		endpoint := fmt.Sprintf("%s/zones/%s/dns_records/%s", c.baseURL, zoneID, existing.ID)
		return c.doRecord(ctx, http.MethodPut, endpoint, payload, apiToken)
	}
	endpoint := fmt.Sprintf("%s/zones/%s/dns_records", c.baseURL, zoneID)
	return c.doRecord(ctx, http.MethodPost, endpoint, payload, apiToken)
}

type dnsRecordRequest struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
	Comment string `json:"comment,omitempty"`
}

// dnsRecord captures the subset of fields we read off responses.
type dnsRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

func (c *Client) findCNAME(ctx context.Context, zoneID, name, apiToken string) (*dnsRecord, error) {
	q := url.Values{}
	q.Set("type", "CNAME")
	q.Set("name", name)
	endpoint := fmt.Sprintf("%s/zones/%s/dns_records?%s", c.baseURL, zoneID, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	c.authorize(req, apiToken)
	resp, err := c.httpC.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cfclient: list records: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, parseAPIError(resp)
	}
	var body struct {
		Success bool        `json:"success"`
		Result  []dnsRecord `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("cfclient: decode records: %w", err)
	}
	if len(body.Result) == 0 {
		return nil, nil
	}
	rec := body.Result[0]
	return &rec, nil
}

func (c *Client) doRecord(ctx context.Context, method, endpoint string, payload dnsRecordRequest, apiToken string) (string, error) {
	buf, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	c.authorize(req, apiToken)
	resp, err := c.httpC.Do(req)
	if err != nil {
		return "", fmt.Errorf("cfclient: %s record: %w", strings.ToLower(method), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", parseAPIError(resp)
	}
	var body struct {
		Success bool      `json:"success"`
		Result  dnsRecord `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("cfclient: decode record: %w", err)
	}
	if !body.Success || body.Result.ID == "" {
		return "", errors.New("cfclient: cloudflare returned no record id")
	}
	if !validCFID(body.Result.ID) {
		return "", errors.New("cfclient: cloudflare returned an unexpected record id format")
	}
	return body.Result.ID, nil
}

// authorize attaches the bearer token. Token is treated as opaque
// material — never logged or echoed back to the caller in error paths.
func (c *Client) authorize(req *http.Request, apiToken string) {
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Accept", "application/json")
}

// parseAPIError extracts a human-readable error from a CF v4 response.
// CF returns:
//
//	{ "success": false, "errors": [ { "code": 6003, "message": "Invalid request headers" } ] }
//
// We surface the first message; tokens are not echoed.
func parseAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var parsed struct {
		Errors []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil && len(parsed.Errors) > 0 {
		return fmt.Errorf("cloudflare API: %s", parsed.Errors[0].Message)
	}
	return fmt.Errorf("cloudflare API: HTTP %d", resp.StatusCode)
}

// apexOf returns the registrable domain for an FQDN. Cloudflare zones
// are registered at the apex; we need to look up by apex even when the
// merchant's storefront lives on a subdomain.
//
// This is a deliberately small heuristic — for any modern eTLD (e.g.
// "co.uk", "com.au") we'd need the public suffix list. The service
// only supports single-segment TLDs in the cloudflare-auto path for
// now; merchants on multi-segment TLDs should fall back to manual
// CNAME and we surface a clear error.
func apexOf(fqdn string) (string, error) {
	fqdn = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(fqdn)), ".")
	parts := strings.Split(fqdn, ".")
	if len(parts) < 2 {
		return "", apperrors.ValidationFailed("domain", "domain is missing a TLD")
	}
	return strings.Join(parts[len(parts)-2:], "."), nil
}
