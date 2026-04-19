package tax_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
)

func TestNameMatch_ExactMatch(t *testing.T) {
	require.Equal(t, tax.NameMatched, tax.CompareNames("Acme Inc", "Acme Inc"))
}

func TestNameMatch_CaseAndWhitespaceNormalized(t *testing.T) {
	require.Equal(t, tax.NameMatched, tax.CompareNames("ACME  INC.", "acme inc"))
}

func TestNameMatch_PunctuationIgnored(t *testing.T) {
	require.Equal(t, tax.NameMatched, tax.CompareNames("Acme, Inc.", "Acme Inc"))
}

func TestNameMatch_LimitedLevenshtein(t *testing.T) {
	require.Equal(t, tax.NameMatched, tax.CompareNames("Acme Widgets Limited", "Acme Widgets Limted"))
}

func TestNameMatch_UnmatchedWhenDistanceTooHigh(t *testing.T) {
	require.Equal(t, tax.NameUnmatched, tax.CompareNames("Acme Widgets Ltd", "Zephyr Holdings Ltd"))
}

func TestNameMatch_EmptyRegistry_ReturnsNotChecked(t *testing.T) {
	require.Equal(t, tax.NameNotChecked, tax.CompareNames("Acme Inc", ""))
	require.Equal(t, tax.NameNotChecked, tax.CompareNames("", "Acme Inc"))
}

func TestNameMatch_CorporateSuffixesEquivalent(t *testing.T) {
	require.Equal(t, tax.NameMatched, tax.CompareNames("Acme Pty Ltd", "Acme Proprietary Limited"))
	require.Equal(t, tax.NameMatched, tax.CompareNames("Acme Sdn Bhd", "Acme Sendirian Berhad"))
}
