package validators_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
	"github.com/mark8ly/marketplace-api/internal/billing/tax/validators"
)

func TestMY_FormatValid_EntersManualReview(t *testing.T) {
	v := validators.NewMY()
	res, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "MY", TaxID: "C12345678901", BusinessName: "Acme Sdn Bhd",
	})
	require.NoError(t, err)
	require.False(t, res.Valid, "manual review means not-yet-valid")
	require.True(t, res.ManualReviewRequired)
	require.Equal(t, "mof_sst_manual", res.QueueReason)
}

func TestMY_BadFormat_Rejected(t *testing.T) {
	v := validators.NewMY()
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "MY", TaxID: "bad",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestMY_WrongCountry_Rejected(t *testing.T) {
	v := validators.NewMY()
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "TH", TaxID: "C12345678901",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestMY_Country(t *testing.T) {
	require.Equal(t, "MY", validators.NewMY().Country())
}
