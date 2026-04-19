package validators

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
)

// AU ABN format: 11 digits.
var abnRegex = regexp.MustCompile(`^\d{11}$`)

// ABRBaseURL is the production ABN Lookup root. The endpoint requires a GUID
// (registered via business.gov.au) injected via WithGUID.
const ABRBaseURL = "https://abr.business.gov.au"

// AUValidator calls the ABR ABN Lookup endpoint.
type AUValidator struct {
	client  *http.Client
	baseURL string
	guid    string // injected via WithGUID
}

// NewAU constructs the validator. nil client → http.DefaultClient, empty
// baseURL → production ABR endpoint.
func NewAU(client *http.Client, baseURL string) *AUValidator {
	if client == nil {
		client = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = ABRBaseURL
	}
	return &AUValidator{client: client, baseURL: baseURL}
}

// WithGUID returns a shallow copy with the ABR Lookup GUID bound.
func (v *AUValidator) WithGUID(guid string) *AUValidator {
	cp := *v
	cp.guid = guid
	return &cp
}

// Country returns ISO-3166 alpha-2.
func (v *AUValidator) Country() string { return "AU" }

type abnResponse struct {
	AbnStatus  string `json:"AbnStatus"`
	EntityName string `json:"EntityName"`
}

// Validate calls AbnDetails.aspx and accepts only "Active" status.
func (v *AUValidator) Validate(ctx context.Context, req tax.ValidationRequest) (tax.ValidationResult, error) {
	if req.Country != "AU" {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	id := strings.ReplaceAll(req.TaxID, " ", "")
	if !abnRegex.MatchString(id) {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}

	q := url.Values{}
	q.Set("abn", id)
	q.Set("guid", v.guid)
	endpoint := fmt.Sprintf("%s/json/AbnDetails.aspx?%s", v.baseURL, q.Encode())

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return tax.ValidationResult{}, fmt.Errorf("au: build request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")

	resp, err := v.client.Do(httpReq)
	if err != nil {
		return tax.ValidationResult{}, tax.ErrRegistryUnavailable
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return tax.ValidationResult{}, tax.ErrNotFound
	case resp.StatusCode >= 500:
		return tax.ValidationResult{}, tax.ErrRegistryUnavailable
	case resp.StatusCode != http.StatusOK:
		return tax.ValidationResult{}, fmt.Errorf("au: unexpected status %d", resp.StatusCode)
	}

	var body abnResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return tax.ValidationResult{}, fmt.Errorf("au: decode response: %w", err)
	}
	if !strings.EqualFold(body.AbnStatus, "Active") {
		return tax.ValidationResult{}, tax.ErrNotFound
	}
	return tax.ValidationResult{
		Valid:        true,
		RegistryName: body.EntityName,
	}, nil
}
