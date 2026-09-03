//go:build integration

package breakglass_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/breakglass"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedRepoAccount is a thin wrapper around seedAccount (defined in
// platform_list_integration_test.go, same package) so this file reads
// standalone.
func seedRepoAccount(t *testing.T, db *gorm.DB, tenantID uuid.UUID) {
	t.Helper()
	seedAccount(t, db, tenantID, nil)
}

// #404: Disable must set both disabled_at and disabled_reason, and Enable
// must clear BOTH — a partial clear (e.g. leaving disabled_reason behind)
// would leave stale forensic text sitting on a re-enabled, otherwise-clean
// account.
func TestIntegration_Repository_DisableThenEnable_RoundTrips(t *testing.T) {
	db := testdb.NewTx(t)
	repo := breakglass.NewRepository(db)
	tenantID := uuid.New()
	seedRepoAccount(t, db, tenantID)

	require.NoError(t, repo.Disable(context.Background(), tenantID, "compromised laptop"))

	acc, err := repo.GetByTenant(context.Background(), tenantID)
	require.NoError(t, err)
	require.NotNil(t, acc.DisabledAt, "Disable must set disabled_at")
	require.WithinDuration(t, time.Now().UTC(), *acc.DisabledAt, 5*time.Second)
	require.NotNil(t, acc.DisabledReason)
	require.Equal(t, "compromised laptop", *acc.DisabledReason)

	require.NoError(t, repo.Enable(context.Background(), tenantID))

	acc, err = repo.GetByTenant(context.Background(), tenantID)
	require.NoError(t, err)
	require.Nil(t, acc.DisabledAt, "Enable must clear disabled_at")
	require.Nil(t, acc.DisabledReason, "Enable must clear disabled_reason too, not just disabled_at")
}

func TestIntegration_Repository_Disable_UnknownTenantReturnsErrNotFound(t *testing.T) {
	db := testdb.NewTx(t)
	repo := breakglass.NewRepository(db)

	err := repo.Disable(context.Background(), uuid.New(), "reason")
	require.ErrorIs(t, err, breakglass.ErrNotFound)
}

func TestIntegration_Repository_Enable_UnknownTenantReturnsErrNotFound(t *testing.T) {
	db := testdb.NewTx(t)
	repo := breakglass.NewRepository(db)

	err := repo.Enable(context.Background(), uuid.New())
	require.ErrorIs(t, err, breakglass.ErrNotFound)
}

// ClearIPLock must remove only ACTIVE rows (locked_until > now()) for the
// given ip_hash — an expired row is already inert, and a row for a
// DIFFERENT ip_hash must survive untouched.
func TestIntegration_Repository_ClearIPLock_RemovesOnlyActiveRowsForThatHash(t *testing.T) {
	db := testdb.NewTx(t)
	repo := breakglass.NewRepository(db)
	now := time.Now().UTC()

	targetHash := breakglass.HMACIPHash([]byte("test-key"), "203.0.113.5")
	otherHash := breakglass.HMACIPHash([]byte("test-key"), "203.0.113.9")

	// Active lockout for the target hash — must be removed.
	require.NoError(t, db.Table("break_glass_lockouts").Create(map[string]interface{}{
		"ip_hash":      targetHash,
		"tenant_id":    nil,
		"locked_until": now.Add(1 * time.Hour),
		"reason":       "3-strike",
	}).Error)
	// Expired lockout for the SAME hash — must survive (already inert).
	require.NoError(t, db.Table("break_glass_lockouts").Create(map[string]interface{}{
		"ip_hash":      targetHash,
		"tenant_id":    nil,
		"locked_until": now.Add(-1 * time.Hour),
		"reason":       "3-strike",
	}).Error)
	// Active lockout for a DIFFERENT hash — must survive.
	require.NoError(t, db.Table("break_glass_lockouts").Create(map[string]interface{}{
		"ip_hash":      otherHash,
		"tenant_id":    nil,
		"locked_until": now.Add(1 * time.Hour),
		"reason":       "3-strike",
	}).Error)

	removed, err := repo.ClearIPLock(context.Background(), targetHash)
	require.NoError(t, err)
	require.EqualValues(t, 1, removed, "only the one ACTIVE row for targetHash must be removed")

	locked, err := repo.IsIPLocked(context.Background(), targetHash)
	require.NoError(t, err)
	require.False(t, locked, "targetHash must no longer be locked after ClearIPLock")

	stillLocked, err := repo.IsIPLocked(context.Background(), otherHash)
	require.NoError(t, err)
	require.True(t, stillLocked, "a different ip_hash's active lockout must not be touched")
}

func TestIntegration_Repository_ClearIPLock_NoActiveRowsRemovesNothing(t *testing.T) {
	db := testdb.NewTx(t)
	repo := breakglass.NewRepository(db)

	removed, err := repo.ClearIPLock(context.Background(), breakglass.HMACIPHash([]byte("k"), "198.51.100.1"))
	require.NoError(t, err)
	require.EqualValues(t, 0, removed)
}
