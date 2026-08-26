//go:build integration

package email_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// Spec §4: "No seed migration ... A key with no row simply renders from its
// embedded default." Registering the billing fallbacks AFTER SeedFromEmbedded
// is what makes that true — main.go's ordering is guarded separately in
// cmd/marketplace-api/wiring_test.go; this asserts the resulting behaviour.
func TestRegisterFallbacksAfterSeed_LeavesNoPublishedRows(t *testing.T) {
	tx := testdb.NewTx(t)
	ctx := context.Background()

	loader := emailtemplates.NewLoader(tx)
	require.NoError(t, loader.SeedFromEmbedded(ctx))
	email.RegisterFallbacks(loader)

	keys := email.BillingTemplateKeys()
	strs := make([]string, len(keys))
	for i, k := range keys {
		strs[i] = string(k)
	}

	var n int64
	require.NoError(t, tx.Raw(
		`SELECT count(*) FROM email_templates WHERE key IN ?`, strs).Scan(&n).Error)
	require.EqualValues(t, 0, n,
		"billing templates were seeded as DB rows — an edit to templates_content.go "+
			"would then never reach a merchant (ON CONFLICT DO NOTHING)")

	// And the embedded default still renders, so nothing is lost.
	rendered, err := loader.Render(ctx, string(email.TemplateDunningDay5), map[string]any{
		"store_name": "Acme", "day": 5,
	})
	require.NoError(t, err)
	require.NotEmpty(t, rendered.Subject)
}

// The inverse, so the ordering's consequence is not merely asserted but
// demonstrated: registering BEFORE the seed does write published rows.
func TestRegisterFallbacksBeforeSeed_WouldSeedPublishedRows(t *testing.T) {
	tx := testdb.NewTx(t)
	ctx := context.Background()

	loader := emailtemplates.NewLoader(tx)
	email.RegisterFallbacks(loader)
	require.NoError(t, loader.SeedFromEmbedded(ctx))

	var n int64
	require.NoError(t, tx.Raw(
		`SELECT count(*) FROM email_templates WHERE key = ? AND status = 'published'`,
		string(email.TemplateDunningDay5)).Scan(&n).Error)
	require.EqualValues(t, 1, n, "expected the wrong ordering to seed a published row")
}
