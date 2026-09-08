//go:build integration

package stores_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// These pin the one behaviour of stores.created_at that has already broken in
// production (#827).
//
// GORM auto-populates any field named CreatedAt on Create, and Upsert is a
// Create with an ON CONFLICT clause. So the column shipped, every upsert began
// sending now(), the ON CONFLICT rule preferred the incoming value, and four
// stores had their real creation dates replaced with the moment of the last
// sync — inside minutes, silently.
//
// The consequence was not cosmetic: the trial backfill dates a trial from this
// column, so a store created in April would have been handed a fresh 90-day
// trial. Only a dry run caught it.
//
// A unit test cannot see any of this. The overwrite happens in SQL GORM
// generates, against a real ON CONFLICT clause.

func upsertStore(t *testing.T, repo stores.Repository, id, tenant uuid.UUID, createdAt *time.Time) {
	t.Helper()
	require.NoError(t, repo.Upsert(context.Background(), &stores.Store{
		ID:           id.String(),
		TenantID:     tenant.String(),
		Slug:         "s-" + id.String()[:8],
		Name:         "Test Store",
		CountryCode:  "AU",
		CurrencyCode: "AUD",
		Timezone:     "Australia/Sydney",
		Status:       "active",
		CreatedAt:    createdAt,
		SyncedAt:     time.Now().UTC(),
	}))
}

func loadStore(t *testing.T, repo stores.Repository, id, tenant uuid.UUID) *stores.Store {
	t.Helper()
	got, err := repo.GetByIDForTenant(context.Background(), id.String(), tenant.String())
	require.NoError(t, err)
	require.NotNil(t, got)
	return got
}

func loadCreatedAt(t *testing.T, repo stores.Repository, id, tenant uuid.UUID) *time.Time {
	t.Helper()
	return loadStore(t, repo, id, tenant).CreatedAt
}

// The regression itself: a caller that sends no created_at must leave the
// stored one alone. Before autoCreateTime:false, GORM filled it with now() and
// the upsert overwrote a real date on every sync.
func TestUpsert_DoesNotOverwriteCreatedAtWhenTheCallerSendsNone(t *testing.T) {
	db := testdb.NewTx(t)
	repo := stores.NewRepository(db)

	id, tenant := uuid.New(), uuid.New()
	original := time.Date(2026, 4, 29, 12, 11, 3, 0, time.UTC)

	upsertStore(t, repo, id, tenant, &original)
	require.WithinDuration(t, original, *loadCreatedAt(t, repo, id, tenant), time.Second)

	// The sync that used to destroy it.
	upsertStore(t, repo, id, tenant, nil)

	got := loadCreatedAt(t, repo, id, tenant)
	require.NotNil(t, got, "created_at was nulled by a sync that did not carry one")
	require.WithinDuration(t, original, *got, time.Second,
		"a sync overwrote the store's real creation date")
}

// The other half of the COALESCE: a row mirrored before migration 136 holds
// NULL and must be able to learn its date from the next sync.
func TestUpsert_FillsCreatedAtWhenTheStoredValueIsNull(t *testing.T) {
	db := testdb.NewTx(t)
	repo := stores.NewRepository(db)

	id, tenant := uuid.New(), uuid.New()
	upsertStore(t, repo, id, tenant, nil)
	require.Nil(t, loadCreatedAt(t, repo, id, tenant))

	known := time.Date(2026, 4, 29, 12, 11, 3, 0, time.UTC)
	upsertStore(t, repo, id, tenant, &known)

	got := loadCreatedAt(t, repo, id, tenant)
	require.NotNil(t, got, "a known creation date did not fill a NULL")
	require.WithinDuration(t, known, *got, time.Second)
}

// created_at and synced_at must not track each other. They did, exactly, when
// the bug was live — which is the tell an operator would have to notice.
func TestUpsert_CreatedAtDoesNotFollowSyncedAt(t *testing.T) {
	db := testdb.NewTx(t)
	repo := stores.NewRepository(db)

	id, tenant := uuid.New(), uuid.New()
	original := time.Date(2026, 4, 29, 12, 11, 3, 0, time.UTC)
	upsertStore(t, repo, id, tenant, &original)

	time.Sleep(10 * time.Millisecond)
	upsertStore(t, repo, id, tenant, nil)

	got := loadStore(t, repo, id, tenant)
	require.NotNil(t, got.CreatedAt)
	require.True(t, got.SyncedAt.Sub(*got.CreatedAt) > time.Hour,
		"created_at moved with synced_at: created=%s synced=%s", got.CreatedAt, got.SyncedAt)
}
