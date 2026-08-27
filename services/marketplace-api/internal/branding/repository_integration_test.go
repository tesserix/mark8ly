//go:build integration

package branding

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// #394: a merchant turning "Powered by Mark8ly" off on their FIRST branding
// save had it silently re-enabled.
//
// Upsert has two paths and only one was protected. The UPDATE path already
// used Select("*") and its comment named show_powered_by explicitly; the
// CREATE path four lines above it was a bare .Create(b), and GORM omits a
// zero-valued field carrying a `default:` tag from the INSERT. `false` is the
// zero value for bool, so the column default true won.
//
// That made it a first-write-only defect: every save after the row existed
// took the protected path and worked, which is why it survived.
func TestUpsert_ShowPoweredByFalseSurvivesFirstWrite(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := NewRepository()
	ctx := context.Background()

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, tx, tenantID, storeID)

	b := defaultBranding(storeID)
	b.TenantID = tenantID
	b.ShowPoweredBy = false

	require.NoError(t, repo.Upsert(ctx, tx, b))

	got, err := repo.GetByStoreID(ctx, tx, storeID)
	require.NoError(t, err)
	require.False(t, got.ShowPoweredBy,
		"show_powered_by=false must survive the CREATE path, not fall back to the column default")

	// And the update path still works, so the fix did not trade one for the
	// other: flip it back on through the second branch of Upsert.
	got.ShowPoweredBy = true
	require.NoError(t, repo.Upsert(ctx, tx, got))

	again, err := repo.GetByStoreID(ctx, tx, storeID)
	require.NoError(t, err)
	require.True(t, again.ShowPoweredBy)
}
