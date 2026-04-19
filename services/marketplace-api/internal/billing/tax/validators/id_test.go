package validators_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
	"github.com/mark8ly/marketplace-api/internal/billing/tax/validators"
)

func TestID_NPWP_PlainFormatValid_EntersManualReview(t *testing.T) {
	v := validators.NewID()
	res, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "ID", TaxID: "012345678901234", BusinessName: "PT Acme",
	})
	require.NoError(t, err)
	require.False(t, res.Valid)
	require.True(t, res.ManualReviewRequired)
	require.Equal(t, "djp_manual", res.QueueReason)
}

func TestID_NPWP_DashedFormatValid(t *testing.T) {
	v := validators.NewID()
	res, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "ID", TaxID: "01.234.567.8-901.234",
	})
	require.NoError(t, err)
	require.True(t, res.ManualReviewRequired)
}

func TestID_NPWP_BadFormat_Rejected(t *testing.T) {
	v := validators.NewID()
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "ID", TaxID: "bad",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestID_WrongCountry_Rejected(t *testing.T) {
	v := validators.NewID()
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "TH", TaxID: "012345678901234",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestID_Country(t *testing.T) {
	require.Equal(t, "ID", validators.NewID().Country())
}
