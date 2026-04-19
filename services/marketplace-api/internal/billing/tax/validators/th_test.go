package validators_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
	"github.com/mark8ly/marketplace-api/internal/billing/tax/validators"
)

func TestTH_FormatValid_EntersManualReview(t *testing.T) {
	v := validators.NewTH()
	res, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "TH", TaxID: "1234567890123", BusinessName: "Acme Co Ltd",
	})
	require.NoError(t, err)
	require.False(t, res.Valid)
	require.True(t, res.ManualReviewRequired)
	require.Equal(t, "rd_manual", res.QueueReason)
}

func TestTH_BadFormat_Rejected(t *testing.T) {
	v := validators.NewTH()
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "TH", TaxID: "12345",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestTH_WrongCountry_Rejected(t *testing.T) {
	v := validators.NewTH()
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "MY", TaxID: "1234567890123",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestTH_Country(t *testing.T) {
	require.Equal(t, "TH", validators.NewTH().Country())
}
