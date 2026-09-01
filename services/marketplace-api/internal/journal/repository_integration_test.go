//go:build integration

package journal_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/journal"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// journal_subscribers has no tenant_id and no FK into stores/tenants — it
// is a platform-level marketing table (see migrations/000124's comment) —
// so unlike most repository integration tests here, there is no
// testdb.SeedStore fixture to set up first.

func TestSubscribe_InsertsANewRow(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := journal.NewRepository(tx)

	email := uuid.New().String() + "@example.com"
	require.NoError(t, repo.Subscribe(email, journal.SourceJournal))

	var count int64
	require.NoError(t, tx.Raw(
		`SELECT count(*) FROM journal_subscribers WHERE email = ?`, email,
	).Scan(&count).Error)
	require.EqualValues(t, 1, count)
}

// The core requirement: submitting the same email twice must succeed both
// times and leave exactly one row — never a 500, never a duplicate.
func TestSubscribe_SameEmailTwiceYieldsOneRow(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := journal.NewRepository(tx)

	email := uuid.New().String() + "@example.com"

	require.NoError(t, repo.Subscribe(email, journal.SourceJournal))
	require.NoError(t, repo.Subscribe(email, journal.SourceJournal), "resubscribing must not error")

	var count int64
	require.NoError(t, tx.Raw(
		`SELECT count(*) FROM journal_subscribers WHERE email = ?`, email,
	).Scan(&count).Error)
	require.EqualValues(t, 1, count, "double submit must not create a duplicate row")
}

// Case variants and surrounding whitespace must collapse to the same
// normalised row, since the unique index is on the normalised value.
func TestSubscribe_NormalizesBeforeInsert(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := journal.NewRepository(tx)

	raw := uuid.New().String()
	mixedCase := "  " + raw + "@Example.COM  "
	normalized := raw + "@example.com"

	require.NoError(t, repo.Subscribe(mixedCase, journal.SourceJournal))
	require.NoError(t, repo.Subscribe(normalized, journal.SourceJournal))

	var count int64
	require.NoError(t, tx.Raw(
		`SELECT count(*) FROM journal_subscribers WHERE email = ?`, normalized,
	).Scan(&count).Error)
	require.EqualValues(t, 1, count)
}
