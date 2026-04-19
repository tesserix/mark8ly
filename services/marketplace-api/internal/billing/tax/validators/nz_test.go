package validators_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
	"github.com/mark8ly/marketplace-api/internal/billing/tax/validators"
)

func TestNZ_Disabled_ReturnsValidatorDisabled(t *testing.T) {
	v := validators.NewNZDisabled()
	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "NZ", TaxID: "123-456-789",
	})
	require.ErrorIs(t, err, tax.ErrValidatorDisabled)
}

func TestNZ_Disabled_Country(t *testing.T) {
	require.Equal(t, "NZ", validators.NewNZDisabled().Country())
}

func TestNZ_Enabled_Constructs(t *testing.T) {
	v := validators.NewNZ(nil, "http://unused")
	require.NotNil(t, v)
	require.Equal(t, "NZ", v.Country())
}
