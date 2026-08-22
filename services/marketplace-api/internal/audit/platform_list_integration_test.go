//go:build integration

package audit_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// ListPlatform counts across every store and tenant, so each test uses
// testdb.NewTx(t) for an isolated, rolled-back view — leftover rows from
// other tests (or other test files sharing the table) would make the Total
// assertions flaky under NewDB/TRUNCATE semantics with parallel packages.
func TestListPlatformSpansStores(t *testing.T) {
	db := testdb.NewTx(t)
	repo := audit.NewRepository()
	ctx := context.Background()

	tenantA, tenantB := uuid.New(), uuid.New()
	storeA, storeB := uuid.New(), uuid.New()

	for _, e := range []*audit.Entry{
		{TenantID: tenantA, StoreID: &storeA, ActorType: audit.ActorUser, Action: "product.deleted", ResourceType: "product", Status: audit.StatusSuccess, Severity: audit.SeverityInfo},
		{TenantID: tenantB, StoreID: &storeB, ActorType: audit.ActorUser, Action: "order.cancelled", ResourceType: "order", Status: audit.StatusSuccess, Severity: audit.SeverityInfo},
		{TenantID: tenantA, StoreID: nil, ActorType: audit.ActorOperator, Action: "tenant.suspended", ResourceType: "tenant", Status: audit.StatusSuccess, Severity: audit.SeverityWarning},
	} {
		require.NoError(t, repo.Create(ctx, db, e))
	}

	got, err := repo.ListPlatform(ctx, db, audit.PlatformListFilter{Limit: 50})
	require.NoError(t, err)
	require.GreaterOrEqual(t, got.Total, int64(3))
	require.GreaterOrEqual(t, len(got.Entries), 3, "rows must span stores and include store-less rows")
}

func TestListPlatformNarrowsByStore(t *testing.T) {
	db := testdb.NewTx(t)
	repo := audit.NewRepository()
	ctx := context.Background()

	tenant, storeA, storeB := uuid.New(), uuid.New(), uuid.New()
	require.NoError(t, repo.Create(ctx, db, &audit.Entry{TenantID: tenant, StoreID: &storeA, ActorType: audit.ActorUser, Action: "a", ResourceType: "x", Status: audit.StatusSuccess, Severity: audit.SeverityInfo}))
	require.NoError(t, repo.Create(ctx, db, &audit.Entry{TenantID: tenant, StoreID: &storeB, ActorType: audit.ActorUser, Action: "b", ResourceType: "x", Status: audit.StatusSuccess, Severity: audit.SeverityInfo}))

	got, err := repo.ListPlatform(ctx, db, audit.PlatformListFilter{StoreID: storeA, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Total)
	require.Equal(t, "a", got.Entries[0].Action)
}

func TestListPlatformClampsLimit(t *testing.T) {
	db := testdb.NewTx(t)
	repo := audit.NewRepository()

	// An oversized limit must clamp, never error — the console is entitled to
	// ask for too much, and a ceiling is our backstop.
	got, err := repo.ListPlatform(context.Background(), db, audit.PlatformListFilter{Limit: 100000})
	require.NoError(t, err)
	require.LessOrEqual(t, len(got.Entries), 500)
}

func TestListPlatformFiltersBySince(t *testing.T) {
	db := testdb.NewTx(t)
	repo := audit.NewRepository()
	ctx := context.Background()
	tenant, store := uuid.New(), uuid.New()

	require.NoError(t, repo.Create(ctx, db, &audit.Entry{TenantID: tenant, StoreID: &store, ActorType: audit.ActorUser, Action: "recent", ResourceType: "x", Status: audit.StatusSuccess, Severity: audit.SeverityInfo}))

	got, err := repo.ListPlatform(ctx, db, audit.PlatformListFilter{
		TenantID: tenant,
		StoreID:  store,
		DateFrom: time.Now().Add(-1 * time.Hour),
		Limit:    50,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, got.Total, int64(1))

	none, err := repo.ListPlatform(ctx, db, audit.PlatformListFilter{
		TenantID: tenant,
		StoreID:  store,
		DateFrom: time.Now().Add(1 * time.Hour),
		Limit:    50,
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), none.Total)
	require.Empty(t, none.Entries)
}

// TestListPlatformActorMatchesOperatorID confirms operator-attributed rows
// (ActorOperatorID set, ActorEmail empty) are findable via the Actor filter,
// which matches partially against actor_email OR actor_operator_id.
func TestListPlatformActorMatchesOperatorID(t *testing.T) {
	db := testdb.NewTx(t)
	repo := audit.NewRepository()
	ctx := context.Background()
	tenant := uuid.New()

	opID := "op_" + uuid.New().String()
	require.NoError(t, repo.Create(ctx, db, &audit.Entry{
		TenantID:        tenant,
		StoreID:         nil,
		ActorType:       audit.ActorOperator,
		ActorOperatorID: &opID,
		Action:          "tenant.suspended",
		ResourceType:    "tenant",
		Status:          audit.StatusSuccess,
		Severity:        audit.SeverityWarning,
	}))

	got, err := repo.ListPlatform(ctx, db, audit.PlatformListFilter{TenantID: tenant, Actor: opID, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Total)
	require.Equal(t, "tenant.suspended", got.Entries[0].Action)
}
