package taxreg_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
	"github.com/mark8ly/marketplace-api/internal/billing/tax/taxreg"
)

func TestBuildDefault_LookupSupportedCountry(t *testing.T) {
	r := taxreg.BuildDefault(taxreg.Config{NZEnabled: false})
	v, ok := r.For("GB")
	require.True(t, ok)
	require.Equal(t, "GB", v.Country())
}

func TestBuildDefault_NZDisabled_ReturnsSentinel(t *testing.T) {
	r := taxreg.BuildDefault(taxreg.Config{NZEnabled: false})
	v, ok := r.For("NZ")
	require.True(t, ok)

	_, err := v.Validate(context.Background(), tax.ValidationRequest{
		Country: "NZ", TaxID: "123-456-789",
	})
	require.ErrorIs(t, err, tax.ErrValidatorDisabled)
}

func TestBuildDefault_NZEnabled_BindsLiveValidator(t *testing.T) {
	r := taxreg.BuildDefault(taxreg.Config{NZEnabled: true})
	v, ok := r.For("NZ")
	require.True(t, ok)
	require.Equal(t, "NZ", v.Country())
}

func TestBuildDefault_UnsupportedCountry(t *testing.T) {
	r := taxreg.BuildDefault(taxreg.Config{})
	_, ok := r.For("ZZ")
	require.False(t, ok)
}

func TestBuildDefault_AllCountriesPresent(t *testing.T) {
	r := taxreg.BuildDefault(taxreg.Config{})
	for _, c := range []string{
		"US", "CA", "GB",
		"IE", "DE", "FR", "IT", "ES", "NL",
		"AU", "NZ", "IN", "SG",
		"MY", "TH", "PH", "ID", "VN",
	} {
		v, ok := r.For(c)
		require.Truef(t, ok, "country %s not registered", c)
		require.Equalf(t, c, v.Country(), "Country() returned %q for %s", v.Country(), c)
	}
}
