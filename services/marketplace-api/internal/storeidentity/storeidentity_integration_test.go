//go:build integration

package storeidentity

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// TestLoad_JoinsBrandingSupportEmail is the reason this package issues a
// LEFT JOIN rather than reading the branding model: support_email is not
// a field on branding.StoreBranding, so only SQL can see it.
func TestLoad_JoinsBrandingSupportEmail(t *testing.T) {
	tx := testdb.NewTx(t)
	ctx := context.Background()

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, tx, tenantID, storeID)
	require.NoError(t, tx.Exec(`
		INSERT INTO store_branding (tenant_id, store_id, support_email)
		VALUES (?, ?, ?)`, tenantID, storeID, "hello@nadiasceramics.com").Error)

	got, err := NewDBLoader(tx).Load(ctx, storeID)
	require.NoError(t, err)
	require.Equal(t, tenantID.String(), got.TenantID)
	require.Equal(t, "Test Store", got.Name)
	require.NotEmpty(t, got.Slug)
	require.Equal(t, "hello@nadiasceramics.com", got.ContactEmail)
}

// TestLoad_NoBrandingRow — the LEFT JOIN must still return the store.
// An INNER JOIN here would silently strip the sender identity from every
// merchant who never opened the branding editor.
func TestLoad_NoBrandingRow(t *testing.T) {
	tx := testdb.NewTx(t)

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, tx, tenantID, storeID)

	got, err := NewDBLoader(tx).Load(context.Background(), storeID)
	require.NoError(t, err)
	require.Equal(t, "Test Store", got.Name)
	require.Empty(t, got.ContactEmail)
}

// TestLoad_BrandingRowWithoutSupportEmail — the column is NOT NULL
// DEFAULT ”, so this is the common case today: no admin surface writes
// it, and Reply-To falls back to the platform address.
func TestLoad_BrandingRowWithoutSupportEmail(t *testing.T) {
	tx := testdb.NewTx(t)

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, tx, tenantID, storeID)
	require.NoError(t, tx.Exec(`
		INSERT INTO store_branding (tenant_id, store_id) VALUES (?, ?)`,
		tenantID, storeID).Error)

	got, err := NewDBLoader(tx).Load(context.Background(), storeID)
	require.NoError(t, err)
	require.Empty(t, got.ContactEmail)
}

// TestLoad_MissingStore must not error — see the Load doc comment.
func TestLoad_MissingStore(t *testing.T) {
	tx := testdb.NewTx(t)

	got, err := NewDBLoader(tx).Load(context.Background(), uuid.New())
	require.NoError(t, err, "a missing store must degrade, not fail the send")
	require.Equal(t, Store{}, got)
}
