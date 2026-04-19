package validators_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
	"github.com/mark8ly/marketplace-api/internal/billing/tax/validators"
)

func TestPH_FormatValid_EntersManualReview(t *testing.T) {
	v := validators.NewPH()
	res, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "PH", TaxID: "123-456-789-000", BusinessName: "Acme Corp",
	})
	require.NoError(t, err)
	require.False(t, res.Valid)
	require.True(t, res.ManualReviewRequired)
	require.Equal(t, "bir_manual", res.QueueReason)
}

func TestPH_FormatValid_NoDashes(t *testing.T) {
	v := validators.NewPH()
	res, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "PH", TaxID: "123456789",
	})
	require.NoError(t, err)
	require.True(t, res.ManualReviewRequired)
}

func TestPH_BadFormat_Rejected(t *testing.T) {
	v := validators.NewPH()
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "PH", TaxID: "bad",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestPH_WrongCountry_Rejected(t *testing.T) {
	v := validators.NewPH()
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "TH", TaxID: "123-456-789",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestPH_Country(t *testing.T) {
	require.Equal(t, "PH", validators.NewPH().Country())
}
