package tax_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
	"github.com/mark8ly/marketplace-api/internal/billing/tax/validators"
)

// The format-only validators (US, CA, and NZ when enabled) consult no registry.
// They must therefore record the name match as "not_checked" rather than
// "matched" — the value store_subscriptions.tax_id_name_match already defaults
// to.
//
// This pins the END STATE rather than the mechanism: what matters is the value
// the orchestrator would write, not how the validator expresses it. Before
// #707 each of these echoed the submitted BusinessName back, so
// CompareNames compared a string with itself and returned NameMatched —
// recording that a registry had confirmed the merchant's name when none was
// ever consulted.
func TestFormatOnlyValidatorsRecordNotChecked(t *testing.T) {
	const submitted = "Acme Inc"

	cases := []struct {
		name    string
		country string
		taxID   string
		v       tax.Validator
	}{
		{"US EIN", "US", "12-3456789", validators.NewUS()},
		{"CA BN", "CA", "123456789", validators.NewCA()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tc.v.Validate(context.Background(), tax.ValidationRequest{
				Country: tc.country, TaxID: tc.taxID, BusinessName: submitted,
			})
			require.NoError(t, err)

			// Still valid: format-only countries are not blocked. Only the
			// false name-match assertion was removed.
			require.True(t, res.Valid, "format check should still pass")

			// This is the line that matters. CompareNames maps an empty
			// registry name to NameNotChecked; an echo would yield NameMatched.
			require.Equal(t, tax.NameNotChecked,
				tax.CompareNames(submitted, res.RegistryName),
				"a validator that consults no registry must not record a name match")
		})
	}
}
