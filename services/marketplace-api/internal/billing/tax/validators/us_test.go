package validators_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
	"github.com/mark8ly/marketplace-api/internal/billing/tax/validators"
)

func TestUS_EIN_ValidFormatAccepted(t *testing.T) {
	v := validators.NewUS()
	res, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "US", TaxID: "12-3456789", BusinessName: "Acme Inc",
	})
	require.NoError(t, err)
	require.True(t, res.Valid)
	// No US registry is consulted, so no registry name may be returned. If this
	// ever echoes the submitted name again, CompareNames compares a string with
	// itself and records a "matched" that never happened (#707).
	require.Empty(t, res.RegistryName)
}

func TestUS_EIN_NoDashAlsoAccepted(t *testing.T) {
	v := validators.NewUS()
	res, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "US", TaxID: "123456789", BusinessName: "Acme Inc",
	})
	require.NoError(t, err)
	require.True(t, res.Valid)
}

func TestUS_EIN_BadFormatRejected(t *testing.T) {
	v := validators.NewUS()
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "US", TaxID: "not-an-ein", BusinessName: "Acme Inc",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestUS_EIN_WrongCountryRejected(t *testing.T) {
	v := validators.NewUS()
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "GB", TaxID: "12-3456789",
	})
	require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestUS_Country(t *testing.T) {
	require.Equal(t, "US", validators.NewUS().Country())
}
