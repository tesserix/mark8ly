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

// The erasure path this whole feature exists for: subscribe, then
// unsubscribe by the token that Subscribe generated, and the row must be
// entirely gone — not soft-deleted, not flagged, gone.
func TestSubscribeThenUnsubscribe_RowIsGone(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := journal.NewRepository(tx)

	email := uuid.New().String() + "@example.com"
	require.NoError(t, repo.Subscribe(email, journal.SourceJournal))

	var token string
	require.NoError(t, tx.Raw(
		`SELECT unsubscribe_token FROM journal_subscribers WHERE email = ?`, email,
	).Scan(&token).Error)
	require.Len(t, token, journal.UnsubscribeTokenLength, "Subscribe must have generated a token")

	require.NoError(t, repo.Unsubscribe(token))

	var count int64
	require.NoError(t, tx.Raw(
		`SELECT count(*) FROM journal_subscribers WHERE email = ?`, email,
	).Scan(&count).Error)
	require.EqualValues(t, 0, count, "the row must be deleted outright, not merely flagged")
}

// Using an already-used (or otherwise unknown) token a second time must
// be a safe no-op: no error, and nothing left to delete the second time
// around.
func TestUnsubscribe_UsingTheSameTokenTwiceIsSafe(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := journal.NewRepository(tx)

	email := uuid.New().String() + "@example.com"
	require.NoError(t, repo.Subscribe(email, journal.SourceJournal))

	var token string
	require.NoError(t, tx.Raw(
		`SELECT unsubscribe_token FROM journal_subscribers WHERE email = ?`, email,
	).Scan(&token).Error)

	require.NoError(t, repo.Unsubscribe(token))
	require.NoError(t, repo.Unsubscribe(token), "unsubscribing with an already-used token must not error")
}

// An unrecognised token must be a safe no-op too, never an error — the
// HTTP layer relies on this to respond identically to every kind of
// token it's handed.
func TestUnsubscribe_UnknownTokenIsSafeNoOp(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := journal.NewRepository(tx)

	unknown := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"[:journal.UnsubscribeTokenLength]
	require.Len(t, unknown, journal.UnsubscribeTokenLength)
	require.NoError(t, repo.Unsubscribe(unknown))
}

// The core "don't break links already sitting in inboxes" requirement:
// re-subscribing an address that already exists must not rotate its
// unsubscribe_token.
func TestSubscribe_RepeatSubscribeDoesNotRotateExistingToken(t *testing.T) {
	tx := testdb.NewTx(t)
	repo := journal.NewRepository(tx)

	email := uuid.New().String() + "@example.com"
	require.NoError(t, repo.Subscribe(email, journal.SourceJournal))

	var originalToken string
	require.NoError(t, tx.Raw(
		`SELECT unsubscribe_token FROM journal_subscribers WHERE email = ?`, email,
	).Scan(&originalToken).Error)

	require.NoError(t, repo.Subscribe(email, journal.SourceJournal), "resubscribing must not error")

	var tokenAfterResubscribe string
	require.NoError(t, tx.Raw(
		`SELECT unsubscribe_token FROM journal_subscribers WHERE email = ?`, email,
	).Scan(&tokenAfterResubscribe).Error)

	require.Equal(t, originalToken, tokenAfterResubscribe,
		"a repeat subscribe must not invalidate an unsubscribe link already sent for this address")
}
