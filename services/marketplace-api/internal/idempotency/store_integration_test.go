//go:build integration

package idempotency_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/idempotency"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// idemNow anchors to the real clock rather than a fixed calendar date:
// Lookup checks expires_at against the DB's live now(), so a hardcoded past
// date would make every saved row look already-expired once enough wall-clock
// time has passed since this file was written. Only the relative offsets
// below (±hours) matter for the assertions.
var idemNow = time.Now().UTC()

// A miss is a miss — not an error, and not an empty body that a caller
// could mistake for a stored response.
func TestLookup_MissReturnsFalse(t *testing.T) {
	db := testdb.NewDB(t, "idempotency_keys")

	body, ok, err := idempotency.Lookup(context.Background(), db, "never-seen")
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, body)
}

// A saved response replays byte-for-byte. The stored body is deliberately
// NOT valid-but-different JSON: byte equality is what the caller needs, so
// a re-marshal that reorders keys would be a defect.
func TestSaveThenLookup_ReplaysTheExactBytes(t *testing.T) {
	db := testdb.NewDB(t, "idempotency_keys")

	key := "key-" + uuid.New().String()
	want := json.RawMessage(`{"store_id":"s1","trial_ends_at":"2026-12-01T00:00:00Z"}`)

	require.NoError(t, idempotency.Save(context.Background(), db,
		key, uuid.New().String(), want, idemNow, idempotency.DefaultTTL))

	got, ok, err := idempotency.Lookup(context.Background(), db, key)
	require.NoError(t, err)
	require.True(t, ok)
	require.JSONEq(t, string(want), string(got))
}

// Saving the same key twice must not error — a retry that races itself
// (two pods, same key) has to converge, not 500.
func TestSave_SameKeyTwiceIsNotAnError(t *testing.T) {
	db := testdb.NewDB(t, "idempotency_keys")

	key := "key-" + uuid.New().String()
	tenant := uuid.New().String()
	first := json.RawMessage(`{"n":1}`)

	require.NoError(t, idempotency.Save(context.Background(), db, key, tenant, first, idemNow, idempotency.DefaultTTL))
	require.NoError(t, idempotency.Save(context.Background(), db, key, tenant,
		json.RawMessage(`{"n":2}`), idemNow, idempotency.DefaultTTL))

	got, ok, err := idempotency.Lookup(context.Background(), db, key)
	require.NoError(t, err)
	require.True(t, ok)
	require.JSONEq(t, string(first), string(got),
		"the FIRST response wins — a replay must not change what an earlier caller was told")
}

// The sweep deletes expired rows and leaves live ones. Both are seeded, on
// the exact boundary: a row expiring exactly at `now` is expired.
func TestSweepExpired_DeletesOnlyExpiredRows(t *testing.T) {
	db := testdb.NewDB(t, "idempotency_keys")

	tenant := uuid.New().String()
	live := "live-" + uuid.New().String()
	dead := "dead-" + uuid.New().String()
	boundary := "edge-" + uuid.New().String()

	require.NoError(t, idempotency.Save(context.Background(), db, live, tenant,
		json.RawMessage(`{"k":"live"}`), idemNow, time.Hour))
	require.NoError(t, idempotency.Save(context.Background(), db, dead, tenant,
		json.RawMessage(`{"k":"dead"}`), idemNow.Add(-48*time.Hour), time.Hour))
	require.NoError(t, idempotency.Save(context.Background(), db, boundary, tenant,
		json.RawMessage(`{"k":"edge"}`), idemNow, 0)) // expires_at == idemNow exactly

	n, err := idempotency.SweepExpired(context.Background(), db, idemNow)
	require.NoError(t, err)
	require.Equal(t, int64(2), n, "the long-expired row AND the one expiring exactly now")

	_, ok, err := idempotency.Lookup(context.Background(), db, live)
	require.NoError(t, err)
	require.True(t, ok, "a live key must survive the sweep")

	_, ok, err = idempotency.Lookup(context.Background(), db, dead)
	require.NoError(t, err)
	require.False(t, ok)
}

// An expired-but-not-yet-swept key must not replay. Expiry is a property of
// the row, not of whether the cron has run recently.
func TestLookup_ExpiredKeyIsAMiss(t *testing.T) {
	db := testdb.NewDB(t, "idempotency_keys")

	key := "stale-" + uuid.New().String()
	require.NoError(t, idempotency.Save(context.Background(), db, key, uuid.New().String(),
		json.RawMessage(`{"k":"stale"}`), idemNow.Add(-48*time.Hour), time.Hour))

	_, ok, err := idempotency.Lookup(context.Background(), db, key)
	require.NoError(t, err)
	require.False(t, ok, "an expired row must not replay just because the sweep has not run")
}
