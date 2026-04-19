package tax_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
)

func TestValidationRequest_Fields(t *testing.T) {
	req := tax.ValidationRequest{
		Country:        "GB",
		TaxID:          "GB123456789",
		BusinessName:   "Acme Ltd",
		BillingAddress: "1 Example St",
	}
	require.Equal(t, "GB", req.Country)
}

func TestErrorSentinels_DistinctIdentity(t *testing.T) {
	require.NotErrorIs(t, tax.ErrInvalidFormat, tax.ErrRegistryUnavailable)
	require.NotErrorIs(t, tax.ErrRegistryUnavailable, tax.ErrNotFound)
	require.NotErrorIs(t, tax.ErrNotFound, tax.ErrManualReviewRequired)
	require.NotErrorIs(t, tax.ErrValidatorDisabled, tax.ErrInvalidFormat)
	require.NotErrorIs(t, tax.ErrNameMismatch, tax.ErrManualReviewRequired)
}
