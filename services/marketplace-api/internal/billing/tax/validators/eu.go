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

// VIESBaseURL is the EU Commission's VIES REST root.
// Full path: /ms/{country}/vat/{vat_number}
const VIESBaseURL = "https://ec.europa.eu/taxation_customs/vies/rest-api"

// euCountries enumerates the 6 supported VIES countries.
var euCountries = map[string]struct{}{
	"IE": {}, "DE": {}, "FR": {}, "IT": {}, "ES": {}, "NL": {},
}

// euVATBareRegex matches the digit/alpha portion of an EU VAT number after the
// country prefix is stripped. VIES validates per-country shape server-side; we
// keep the client-side regex broad (8-12 alphanumerics) and let the registry
// reject malformed numbers via a 400/200-with-valid:false response. Anchored
// to ensure no embedded whitespace slips through.
var euVATBareRegex = regexp.MustCompile(`^[A-Z0-9]{8,12}$`)

// EUValidator calls VIES and supports all 6 EU countries via WithCountry.
type EUValidator struct {
	client  *http.Client
	baseURL string
	country string // "" until WithCountry binds a copy
}

// NewEU constructs an unbound EU validator. nil client → http.DefaultClient,
// empty baseURL → VIES production endpoint.
func NewEU(client *http.Client, baseURL string) *EUValidator {
	if client == nil {
		client = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = VIESBaseURL
	}
	return &EUValidator{client: client, baseURL: baseURL}
}

// WithCountry returns a shallow copy bound to the given ISO-3166 alpha-2 code.
func (v *EUValidator) WithCountry(c string) *EUValidator {
	cp := *v
	cp.country = strings.ToUpper(c)
	return &cp
}

// Country returns the bound country (empty until WithCountry is called).
func (v *EUValidator) Country() string { return v.country }

type viesResponse struct {
	Valid   bool   `json:"valid"`
	Name    string `json:"name"`
	Address string `json:"address"`
}

// Validate calls VIES /ms/{country}/vat/{vat_number}.
//
//   - valid=false → ErrNotFound
//   - name="" but valid=true → privacy-protected (NO_INFORMATION); Valid:true,
//     RegistryName:""
//   - 503/504 → ErrRegistryUnavailable explicitly (member-state outage)
func (v *EUValidator) Validate(ctx context.Context, req tax.ValidationRequest) (tax.ValidationResult, error) {
	country := strings.ToUpper(req.Country)
	if v.country != "" && country != v.country {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	if _, ok := euCountries[country]; !ok {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}

	id := strings.ToUpper(strings.ReplaceAll(req.TaxID, " ", ""))
	id = strings.TrimPrefix(id, country) // strip optional country prefix
	if !euVATBareRegex.MatchString(id) {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}

	endpoint := fmt.Sprintf("%s/ms/%s/vat/%s", v.baseURL, country, id)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return tax.ValidationResult{}, fmt.Errorf("eu: build request: %w", err)
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
	case resp.StatusCode == http.StatusServiceUnavailable, resp.StatusCode == http.StatusGatewayTimeout:
		return tax.ValidationResult{}, tax.ErrRegistryUnavailable
	case resp.StatusCode >= 500:
		return tax.ValidationResult{}, tax.ErrRegistryUnavailable
	case resp.StatusCode != http.StatusOK:
		return tax.ValidationResult{}, fmt.Errorf("eu: unexpected status %d", resp.StatusCode)
	}

	var body viesResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return tax.ValidationResult{}, fmt.Errorf("eu: decode response: %w", err)
	}

	if !body.Valid {
		return tax.ValidationResult{}, tax.ErrNotFound
	}
	// name="" => NO_INFORMATION (privacy-protected member state). Mark valid
	// but leave RegistryName empty so the orchestrator records "not_checked".
	return tax.ValidationResult{
		Valid:        true,
		RegistryName: body.Name,
	}, nil
}
