package validators

import (
	"context"
	"regexp"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
)

// MY SST registration: single alpha prefix (W/C/B/J) + 10-11 digits.
// §19.3 MY row: all valid-format SST IDs enter 5-biz-day manual review until
// MOF exposes a public API. Queue entry pauses the clock per §5.2.
var mySSTRegex = regexp.MustCompile(`^[WCBJ]\d{10,11}$`)

// MYValidator enqueues every well-formed Malaysian SST ID for manual review.
type MYValidator struct{}

// NewMY constructs the manual-review-only Malaysian validator.
func NewMY() *MYValidator { return &MYValidator{} }

// Country returns ISO-3166 alpha-2.
func (v *MYValidator) Country() string { return "MY" }

// Validate format-checks then queues for manual review.
func (v *MYValidator) Validate(_ context.Context, req tax.ValidationRequest) (tax.ValidationResult, error) {
	if req.Country != "MY" {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	if !mySSTRegex.MatchString(req.TaxID) {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	return tax.ValidationResult{
		Valid:                false,
		ManualReviewRequired: true,
		QueueReason:          "mof_sst_manual",
	}, nil
}
