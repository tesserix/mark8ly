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

// SG UEN three accepted forms (per ACRA developer docs):
//
//	NNNNNNNNL    — 8 digits + check letter (Business under Business Names Act)
//	NNNNNNNNNL   — 9 digits + check letter (Local Companies)
//	[STRFR]NNALAANNNNL — entity-type prefix + year + entity code + sequence
//
// We accept all three, normalised to upper case.
var uenRegex = regexp.MustCompile(`^(\d{8}[A-Z]|\d{9}[A-Z]|[STRFR]\d{2}[A-Z]{2}\d{4}[A-Z])$`)

// ACRABaseURL is the production ACRA UEN lookup root.
const ACRABaseURL = "https://api.acra.gov.sg"

// SGValidator calls ACRA's UEN endpoint.
type SGValidator struct {
	client  *http.Client
	baseURL string
}

// NewSG constructs the validator.
func NewSG(client *http.Client, baseURL string) *SGValidator {
	if client == nil {
		client = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = ACRABaseURL
	}
	return &SGValidator{client: client, baseURL: baseURL}
}

// Country returns ISO-3166 alpha-2.
func (v *SGValidator) Country() string { return "SG" }

type acraResponse struct {
	EntityStatus string `json:"entityStatus"`
	EntityName   string `json:"entityName"`
}

// Validate calls /v1/uen/{uen}; only "Registered" is accepted.
func (v *SGValidator) Validate(ctx context.Context, req tax.ValidationRequest) (tax.ValidationResult, error) {
	if req.Country != "SG" {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	id := strings.ToUpper(strings.ReplaceAll(req.TaxID, " ", ""))
	if !uenRegex.MatchString(id) {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}

	endpoint := fmt.Sprintf("%s/v1/uen/%s", v.baseURL, id)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return tax.ValidationResult{}, fmt.Errorf("sg: build request: %w", err)
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
		return tax.ValidationResult{}, fmt.Errorf("sg: unexpected status %d", resp.StatusCode)
	}

	var body acraResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return tax.ValidationResult{}, fmt.Errorf("sg: decode response: %w", err)
	}
	if !strings.EqualFold(body.EntityStatus, "Registered") {
		return tax.ValidationResult{}, tax.ErrNotFound
	}
	return tax.ValidationResult{
		Valid:        true,
		RegistryName: body.EntityName,
	}, nil
}
