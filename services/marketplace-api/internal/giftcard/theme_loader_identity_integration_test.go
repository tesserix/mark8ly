//go:build integration

package giftcard

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/branding"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestIntegration_LoadTheme_CarriesStoreSenderIdentity pins the hop the
// mailer-level test in internal/email cannot see (#718): that test hands
// the mailer a theme it built itself, so it proves the mailer APPLIES an
// identity, not that the loader SUPPLIES one. Emptying the two
// assignments in LoadTheme left every other test green.
func TestIntegration_LoadTheme_CarriesStoreSenderIdentity(t *testing.T) {
	tx := testdb.NewTx(t)

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, tx, tenantID, storeID)
	require.NoError(t, tx.Exec(`
		INSERT INTO store_branding (tenant_id, store_id, support_email)
		VALUES (?, ?, ?)`, tenantID, storeID, "hello@nadiasceramics.com").Error)

	loader := NewStoreThemeLoader(tx, branding.NewService(branding.ServiceConfig{DB: tx, Repo: branding.NewRepository()}), "")
	theme, _, err := loader.LoadTheme(context.Background(), storeID)
	require.NoError(t, err)

	require.Equal(t, "Test Store", theme.StoreName)
	require.NotEmpty(t, theme.StoreSlug,
		"the envelope's From local part is derived from this")
	require.Equal(t, "hello@nadiasceramics.com", theme.StoreContactEmail,
		"the envelope's Reply-To comes from this")
}
