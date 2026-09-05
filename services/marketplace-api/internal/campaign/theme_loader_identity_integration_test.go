//go:build integration

package campaign

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/branding"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestIntegration_LoadTheme_CarriesStoreSenderIdentity — see the twin in
// internal/giftcard. The campaign send worker reads these two fields
// straight into the outbound envelope (#718), so a loader that stops
// populating them silently reverts campaigns to the platform sender.
func TestIntegration_LoadTheme_CarriesStoreSenderIdentity(t *testing.T) {
	tx := testdb.NewTx(t)

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, tx, tenantID, storeID)
	require.NoError(t, tx.Exec(`
		INSERT INTO store_branding (tenant_id, store_id, support_email)
		VALUES (?, ?, ?)`, tenantID, storeID, "hello@nadiasceramics.com").Error)

	loader := NewStoreThemeLoader(tx, branding.NewService(branding.ServiceConfig{DB: tx, Repo: branding.NewRepository()}))
	theme, err := loader.LoadTheme(context.Background(), storeID)
	require.NoError(t, err)

	require.Equal(t, "Test Store", theme.StoreName)
	require.NotEmpty(t, theme.StoreSlug)
	require.Equal(t, "hello@nadiasceramics.com", theme.StoreContactEmail)
}
