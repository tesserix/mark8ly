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
