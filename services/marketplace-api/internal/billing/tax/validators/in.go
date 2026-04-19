package validators

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
)

// GSTIN format: 15 chars.
//
//	1-2  state code (01-37)
//	3-12 PAN (5 alpha + 4 digits + 1 alpha)
//	13   entity number (alphanumeric)
//	14   'Z' (reserved)
//	15   checksum (alphanumeric)
var gstinRegex = regexp.MustCompile(`^(0[1-9]|[1-2]\d|3[0-7])[A-Z]{5}\d{4}[A-Z][1-9A-Z]Z[0-9A-Z]$`)

// GSTNBaseURL is the production GSTN commonapi root.
const GSTNBaseURL = "https://api.gst.gov.in"

// INValidator calls the GSTN commonapi search endpoint.
type INValidator struct {
	client    *http.Client
	baseURL   string
	authToken string // injected via WithAuthToken; never logged
}

// NewIN constructs the validator. nil client → http.DefaultClient, empty
// baseURL → production GSTN endpoint.
func NewIN(client *http.Client, baseURL string) *INValidator {
	if client == nil {
		client = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = GSTNBaseURL
	}
	return &INValidator{client: client, baseURL: baseURL}
}

// WithAuthToken returns a shallow copy with the bearer token bound. The token
// lives in Secret Manager and is injected at service construction.
func (v *INValidator) WithAuthToken(token string) *INValidator {
	cp := *v
	cp.authToken = token
	return &cp
}

// Country returns ISO-3166 alpha-2.
func (v *INValidator) Country() string { return "IN" }

type gstnSearchResponse struct {
	Data struct {
		GSTIN string `json:"gstin"`
		LGNM  string `json:"lgnm"` // legal name
		STS   string `json:"sts"`  // status
	} `json:"data"`
}

// Validate calls the GSTN search endpoint. Only "Active" status is accepted.
func (v *INValidator) Validate(ctx context.Context, req tax.ValidationRequest) (tax.ValidationResult, error) {
	if req.Country != "IN" {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	id := strings.ToUpper(strings.ReplaceAll(req.TaxID, " ", ""))
	if !gstinRegex.MatchString(id) {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}

	endpoint := fmt.Sprintf("%s/commonapi/v1.1/search?gstin=%s", v.baseURL, id)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return tax.ValidationResult{}, fmt.Errorf("in: build request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")
	if v.authToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+v.authToken)
	}

	resp, err := v.client.Do(httpReq)
	if err != nil {
		return tax.ValidationResult{}, tax.ErrRegistryUnavailable
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return tax.ValidationResult{}, tax.ErrNotFound
	case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode >= 500:
		return tax.ValidationResult{}, tax.ErrRegistryUnavailable
	case resp.StatusCode != http.StatusOK:
		return tax.ValidationResult{}, fmt.Errorf("in: unexpected status %d", resp.StatusCode)
	}

	var body gstnSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return tax.ValidationResult{}, fmt.Errorf("in: decode response: %w", err)
	}

	if !strings.EqualFold(body.Data.STS, "Active") {
		return tax.ValidationResult{}, tax.ErrNotFound
	}

	return tax.ValidationResult{
		Valid:        true,
		RegistryName: body.Data.LGNM,
	}, nil
}
