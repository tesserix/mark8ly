package validators

import (
	"context"
	"regexp"
	"strings"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
)

// Canada Business Number: 9 digits, optional GST/HST program account suffix
// "RT" + 4 digits. No federal API exposed publicly for BN lookup; like the US
// validator this is format-only + attestation (§19.3.1).
var caBNRegex = regexp.MustCompile(`^\d{9}(RT\d{4})?$`)

// CAValidator is a format-only Business Number validator.
type CAValidator struct{}

// NewCA constructs the format-only Canadian validator.
func NewCA() *CAValidator { return &CAValidator{} }

// Country returns ISO-3166 alpha-2.
func (v *CAValidator) Country() string { return "CA" }

// Validate checks BN shape only, mirroring BusinessName as RegistryName.
func (v *CAValidator) Validate(_ context.Context, req tax.ValidationRequest) (tax.ValidationResult, error) {
	if req.Country != "CA" {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	id := strings.ToUpper(strings.ReplaceAll(req.TaxID, " ", ""))
	if !caBNRegex.MatchString(id) {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	return tax.ValidationResult{
		Valid:        true,
		RegistryName: req.BusinessName,
	}, nil
}
