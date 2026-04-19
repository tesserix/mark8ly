package validators

import (
	"context"
	"regexp"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
)

// VN GDT tax ID: 10 digits, optionally with branch suffix -NNN.
// §19.3 VN row: manual review.
var vnGDTRegex = regexp.MustCompile(`^\d{10}(-\d{3})?$`)

// VNValidator enqueues every well-formed Vietnamese tax ID for manual review.
type VNValidator struct{}

// NewVN constructs the manual-review-only Vietnamese validator.
func NewVN() *VNValidator { return &VNValidator{} }

// Country returns ISO-3166 alpha-2.
func (v *VNValidator) Country() string { return "VN" }

// Validate format-checks then queues for manual review.
func (v *VNValidator) Validate(_ context.Context, req tax.ValidationRequest) (tax.ValidationResult, error) {
	if req.Country != "VN" {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	if !vnGDTRegex.MatchString(req.TaxID) {
		return tax.ValidationResult{}, tax.ErrInvalidFormat
	}
	return tax.ValidationResult{
		Valid:                false,
		ManualReviewRequired: true,
		QueueReason:          "gdt_manual",
	}, nil
}
