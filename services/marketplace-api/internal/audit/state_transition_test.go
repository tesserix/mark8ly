//go:build integration

package audit_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestEmitStateTransition_WritesCanonicalEvent(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs")
	em := audit.NewEmitter(audit.EmitterConfig{
		DB:     db,
		Repo:   audit.NewRepository(),
		Logger: testLogger(t),
	})

	tenantID := uuid.New()
	storeID := uuid.New()

	em.EmitStateTransition(nil, audit.StateTransition{
		StoreID:  storeID,
		TenantID: tenantID,
		From:     "trialing",
		To:       "active",
		Actor:    "system:webhook:stripe",
		Reason:   "invoice.paid",
	})

	// Drain queue.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	em.Stop(ctx)

	var entry audit.Entry
	err := db.Where("tenant_id = ? AND store_id = ?", tenantID, storeID).First(&entry).Error
	require.NoError(t, err, "audit row must be written")

	require.Equal(t, "subscription.state_transition", entry.Action)
	require.Equal(t, "subscription", entry.ResourceType)
	require.NotNil(t, entry.ResourceID)
	require.Equal(t, storeID.String(), *entry.ResourceID)
	require.Equal(t, audit.ActorSystem, entry.ActorType)
	require.Equal(t, audit.SeverityInfo, entry.Severity)

	md := entry.Metadata
	require.Equal(t, "trialing", md["from_status"])
	require.Equal(t, "active", md["to_status"])
	require.Equal(t, "invoice.paid", md["reason"])
	require.Equal(t, "system:webhook:stripe", md["actor"])
}

func TestEmitStateTransition_TerminalStatesGetWarning(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs")
	em := audit.NewEmitter(audit.EmitterConfig{
		DB:     db,
		Repo:   audit.NewRepository(),
		Logger: testLogger(t),
	})

	tenantID := uuid.New()
	storeID := uuid.New()

	em.EmitStateTransition(nil, audit.StateTransition{
		StoreID:  storeID,
		TenantID: tenantID,
		From:     "active",
		To:       "hard_deleted",
		Actor:    "system:cron:retention",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	em.Stop(ctx)

	var entry audit.Entry
	err := db.Where("tenant_id = ? AND store_id = ?", tenantID, storeID).First(&entry).Error
	require.NoError(t, err)
	require.Equal(t, audit.SeverityWarning, entry.Severity)
}

func testLogger(t *testing.T) *slog.Logger {
	return slog.New(slog.NewTextHandler(&testWriter{t: t}, nil))
}

type testWriter struct{ t *testing.T }

func (w *testWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}
