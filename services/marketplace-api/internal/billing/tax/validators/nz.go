package validators

import (
	"context"
	"net/http"
	"regexp"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
)

// NZ IRD number: 8 or 9 digits, optionally dash-separated as XXX-XXX-XX(X).
var nzIRDRegex = regexp.MustCompile(`^\d{3}-?\d{3}-?\d{2,3}$`)

// IRDBaseURL — IRD's public validation endpoint. Placeholder until counsel
// confirms whether the B2B reverse-charge model works under NZ GST rules
// (§20.3). Implementation skeleton is ready; the feature flag gates activation.
const IRDBaseURL = "https://gateway.ird.govt.nz"

// NZValidator is the live NZ implementation. Currently format-only because the
// IRD endpoint requires counsel sign-off (§20.3). When live, this will call IRD.
type NZValidator struct {
	client  *http.Client
	baseURL string
}

// NewNZ constructs the (currently format-only) NZ validator.
func NewNZ(client *http.Client, baseURL string) *NZValidator {
	if client == nil {
		client = http.DefaultClient
	}
	if baseURL == "" {
		baseURL = IRDBaseURL
	}
	return &NZValidator{client: client, baseURL: baseURL}
}

// Country returns ISO-3166 alpha-2.
func (v *NZValidator) Country() string { return "NZ" }

// Validate currently format-checks only; mirrors BusinessName as RegistryName.
// TODO(counsel §20.3): wire IRD lookup once legal sign-off lands.
func (v *NZValidator) Validate(_ context.Context, req tax.ValidationRequest) (tax.ValidationResult, error) {
	if req.Country != "NZ" {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	if !nzIRDRegex.MatchString(req.TaxID) {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	return tax.ValidationResult{Valid: true, RegistryName: req.BusinessName}, nil
}

// NZDisabled is wired by Registry when NZ_TAX_VALIDATION_ENABLED=false. The
// orchestrator maps ErrValidatorDisabled → HTTP 503 with a merchant-friendly
// message.
type NZDisabled struct{}

// NewNZDisabled constructs the always-disabled NZ validator.
func NewNZDisabled() *NZDisabled { return &NZDisabled{} }

// Country returns ISO-3166 alpha-2.
func (*NZDisabled) Country() string { return "NZ" }

// Validate always returns ErrValidatorDisabled.
func (*NZDisabled) Validate(_ context.Context, _ tax.ValidationRequest) (tax.ValidationResult, error) {
	return tax.ValidationResult{}, tax.ErrValidatorDisabled
}
