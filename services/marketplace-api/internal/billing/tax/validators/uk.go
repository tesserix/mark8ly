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

// UK VAT number format: "GB" + 9 digits (standard) or 12 digits (branches).
var ukVATRegex = regexp.MustCompile(`^GB\d{9}(\d{3})?$`)

// HMRCBaseURL is the production HMRC "Check a UK VAT number" API root.
// Full path: /organisations/vat/check-vat-number/lookup/{vrn}
const HMRCBaseURL = "https://api.service.hmrc.gov.uk"

// UKValidator calls HMRC's public Check VAT Number endpoint.
type UKValidator struct {
	client  *http.Client
	baseURL string
}

// NewUK constructs the validator. nil client falls back to http.DefaultClient,
// empty baseURL falls back to the production HMRC endpoint.
func NewUK(client *http.Client, baseURL string) *UKValidator {
	if client == nil {
		client = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = HMRCBaseURL
	}
	return &UKValidator{client: client, baseURL: baseURL}
}

// Country returns ISO-3166 alpha-2.
func (v *UKValidator) Country() string { return "GB" }

type hmrcLookupResponse struct {
	Target struct {
		Name      string `json:"name"`
		VATNumber string `json:"vatNumber"`
	} `json:"target"`
}

// Validate calls HMRC and maps responses to the standard sentinel errors.
func (v *UKValidator) Validate(ctx context.Context, req tax.ValidationRequest) (tax.ValidationResult, error) {
	if req.Country != "GB" {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	id := strings.ToUpper(strings.ReplaceAll(req.TaxID, " ", ""))
	if !ukVATRegex.MatchString(id) {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	vrn := strings.TrimPrefix(id, "GB")

	endpoint, err := url.Parse(v.baseURL)
	if err != nil {
		return tax.ValidationResult{}, fmt.Errorf("uk: parse base url: %w", err)
	}
	endpoint.Path = fmt.Sprintf("/organisations/vat/check-vat-number/lookup/GB%s", vrn)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return tax.ValidationResult{}, fmt.Errorf("uk: build request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/vnd.hmrc.2.0+json")

	resp, err := v.client.Do(httpReq)
	if err != nil {
		// Timeout, DNS failure, TLS error — all treated as outage.
		return tax.ValidationResult{}, tax.ErrRegistryUnavailable
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return tax.ValidationResult{}, tax.ErrNotFound
	case resp.StatusCode >= 500:
		return tax.ValidationResult{}, tax.ErrRegistryUnavailable
	case resp.StatusCode != http.StatusOK:
		return tax.ValidationResult{}, fmt.Errorf("uk: unexpected status %d", resp.StatusCode)
	}

	var body hmrcLookupResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return tax.ValidationResult{}, fmt.Errorf("uk: decode response: %w", err)
	}

	return tax.ValidationResult{
		Valid:        true,
		RegistryName: body.Target.Name,
	}, nil
}
