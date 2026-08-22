//go:build integration

package audit_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// Platform-originated writes (tenant suspend, trial extend, purge) are
// tenant-scoped and carry no store. Before this task they were silently
// dropped by resolveScope. The assertion that matters is that a row exists
// at all.
func TestCreateAcceptsStorelessEntry(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs")
	repo := audit.NewRepository()

	e := &audit.Entry{
		TenantID:     uuid.New(),
		StoreID:      nil,
		ActorType:    audit.ActorSystem,
		Action:       "tenant.suspended",
		ResourceType: "tenant",
		Status:       audit.StatusSuccess,
		Severity:     audit.SeverityWarning,
	}

	require.NoError(t, repo.Create(context.Background(), db, e))
	require.NotEqual(t, uuid.Nil, e.ID)

	var loaded audit.Entry
	require.NoError(t, db.First(&loaded, "id = ?", e.ID).Error)
	require.Nil(t, loaded.StoreID)
}

// A zero-value store pointer (&uuid.Nil) would insert an all-zeros store
// rather than SQL NULL, defeating the point of making the column nullable.
// resolveScope never produces this today, but Create defends against it
// directly since it's the last line of defense before the DB.
func TestCreateRejectsZeroValueStorePointer(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs")
	repo := audit.NewRepository()

	zero := uuid.Nil
	e := &audit.Entry{
		TenantID:     uuid.New(),
		StoreID:      &zero,
		ActorType:    audit.ActorSystem,
		Action:       "tenant.suspended",
		ResourceType: "tenant",
		Status:       audit.StatusSuccess,
		Severity:     audit.SeverityWarning,
	}

	err := repo.Create(context.Background(), db, e)
	require.Error(t, err)
	require.Contains(t, err.Error(), "store_id must be nil or a real store")
}

