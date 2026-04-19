package validators_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
	"github.com/mark8ly/marketplace-api/internal/billing/tax/validators"
)

func TestVN_FormatValid_EntersManualReview(t *testing.T) {
	v := validators.NewVN()
	res, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "VN", TaxID: "1234567890", BusinessName: "Acme Vietnam JSC",
	})
	require.NoError(t, err)
	require.False(t, res.Valid)
	require.True(t, res.ManualReviewRequired)
	require.Equal(t, "gdt_manual", res.QueueReason)
}

func TestVN_FormatValid_WithBranchSuffix(t *testing.T) {
	v := validators.NewVN()
	res, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "VN", TaxID: "1234567890-001",
	})
	require.NoError(t, err)
	require.True(t, res.ManualReviewRequired)
}

func TestVN_BadFormat_Rejected(t *testing.T) {
	v := validators.NewVN()
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "VN", TaxID: "bad",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestVN_WrongCountry_Rejected(t *testing.T) {
	v := validators.NewVN()
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "TH", TaxID: "1234567890",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestVN_Country(t *testing.T) {
	require.Equal(t, "VN", validators.NewVN().Country())
}
