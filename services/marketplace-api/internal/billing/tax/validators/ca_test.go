package validators_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
	"github.com/mark8ly/marketplace-api/internal/billing/tax/validators"
)

func TestCA_BN_ValidFormatAccepted(t *testing.T) {
	v := validators.NewCA()
	res, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "CA", TaxID: "123456789", BusinessName: "Acme Canada Inc",
	})
	require.NoError(t, err)
	require.True(t, res.Valid)
	require.Equal(t, "Acme Canada Inc", res.RegistryName)
}

func TestCA_BN_WithRTSuffixAccepted(t *testing.T) {
	v := validators.NewCA()
	res, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "CA", TaxID: "123456789RT0001", BusinessName: "Acme Canada Inc",
	})
	require.NoError(t, err)
	require.True(t, res.Valid)
}

func TestCA_BN_BadFormatRejected(t *testing.T) {
	v := validators.NewCA()
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "CA", TaxID: "not-a-bn",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestCA_BN_WrongCountryRejected(t *testing.T) {
	v := validators.NewCA()
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "US", TaxID: "123456789",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestCA_Country(t *testing.T) {
	require.Equal(t, "CA", validators.NewCA().Country())
}
