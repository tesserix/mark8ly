//go:build integration

package breakglass_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/breakglass"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func dbKey(b string) breakglass.LoginKey {
	return breakglass.LoginKey{IPHash: []byte(b)}
}

// The property the whole change exists for: the count is a property of the
// ESTATE, so a second "pod" reading the same table sees the first one's
// failures. Two repositories over one database stand in for two replicas —
// with the in-memory window this test is impossible to write at all.
func TestIntegration_DBLoginWindow_CountIsSharedAcrossPods(t *testing.T) {
	db := testdb.NewTx(t)
	podA := breakglass.NewDBLoginWindow(breakglass.NewRepository(db))
	podB := breakglass.NewDBLoginWindow(breakglass.NewRepository(db))
	ctx := context.Background()
	k := dbKey("shared-ip-hash-0001")

	n, err := podA.RecordFailure(ctx, k)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	// The second failure lands on the OTHER pod and must still count as the
	// second, not as that pod's first. This is precisely what per-pod
	// counting got wrong, and why 2 replicas used to mean 6 strikes.
	n, err = podB.RecordFailure(ctx, k)
	require.NoError(t, err)
	require.Equal(t, 2, n)

	n, err = podB.RecordFailure(ctx, k)
	require.NoError(t, err)
	require.GreaterOrEqual(t, n, breakglass.LoginMaxFailures,
		"the threshold must be reached on the third failure regardless of which pod saw them")
}

// Clearing on one pod clears for every pod — the clear-lockout path depends
// on this, or an operator clears a lockout and the IP re-locks on its next
// failure because the attempt rows survived.
func TestIntegration_DBLoginWindow_ResetIsVisibleToEveryPod(t *testing.T) {
	db := testdb.NewTx(t)
	podA := breakglass.NewDBLoginWindow(breakglass.NewRepository(db))
	podB := breakglass.NewDBLoginWindow(breakglass.NewRepository(db))
	ctx := context.Background()
	k := dbKey("shared-ip-hash-0002")

	_, err := podA.RecordFailure(ctx, k)
	require.NoError(t, err)
	_, err = podA.RecordFailure(ctx, k)
	require.NoError(t, err)

	require.NoError(t, podB.Reset(ctx, k))

	n, err := podA.Count(ctx, k)
	require.NoError(t, err)
	require.Zero(t, n, "a reset on one pod must clear the window for all of them")
}

// Two ip_hashes must not share a bucket. A predicate that dropped the
// ip_hash filter would still pass every count test above.
func TestIntegration_DBLoginWindow_CountsPerIPHash(t *testing.T) {
	db := testdb.NewTx(t)
	w := breakglass.NewDBLoginWindow(breakglass.NewRepository(db))
	ctx := context.Background()

	_, err := w.RecordFailure(ctx, dbKey("ip-hash-aaaa"))
	require.NoError(t, err)
	_, err = w.RecordFailure(ctx, dbKey("ip-hash-aaaa"))
	require.NoError(t, err)

	n, err := w.Count(ctx, dbKey("ip-hash-bbbb"))
	require.NoError(t, err)
	require.Zero(t, n, "one IP's failures must not count against another's")
}

// Rows outside LoginRateWindow are ignored. Without this the window is not a
// window at all — an IP that failed twice a year ago would be one strike from
// a 24h lockout forever.
func TestIntegration_DBLoginWindow_IgnoresAttemptsOutsideTheWindow(t *testing.T) {
	db := testdb.NewTx(t)
	repo := breakglass.NewRepository(db)
	w := breakglass.NewDBLoginWindow(repo)
	ctx := context.Background()
	k := dbKey("ip-hash-stale-001")

	_, err := w.RecordFailure(ctx, k)
	require.NoError(t, err)

	// Age the row past the window.
	require.NoError(t, db.Exec(
		`UPDATE break_glass_login_attempts SET attempted_at = ? WHERE ip_hash = ?`,
		time.Now().UTC().Add(-2*breakglass.LoginRateWindow), []byte(k.IPHash),
	).Error)

	n, err := w.Count(ctx, k)
	require.NoError(t, err)
	require.Zero(t, n, "an attempt older than LoginRateWindow is outside the window")
}

// The sweep removes aged rows without touching in-window ones.
func TestIntegration_DBLoginWindow_PruneKeepsInWindowAttempts(t *testing.T) {
	db := testdb.NewTx(t)
	repo := breakglass.NewRepository(db)
	ctx := context.Background()
	fresh := dbKey("ip-hash-fresh-01")
	stale := dbKey("ip-hash-stale-02")

	_, err := repo.RecordLoginFailure(ctx, fresh.IPHash)
	require.NoError(t, err)
	_, err = repo.RecordLoginFailure(ctx, stale.IPHash)
	require.NoError(t, err)
	require.NoError(t, db.Exec(
		`UPDATE break_glass_login_attempts SET attempted_at = ? WHERE ip_hash = ?`,
		time.Now().UTC().Add(-2*breakglass.LoginRateWindow), []byte(stale.IPHash),
	).Error)

	removed, err := repo.PruneLoginAttempts(ctx, breakglass.LoginRateWindow)
	require.NoError(t, err)
	require.Equal(t, int64(1), removed)

	n, err := repo.CountLoginFailures(ctx, fresh.IPHash)
	require.NoError(t, err)
	require.Equal(t, 1, n, "prune must not remove an in-window attempt")
}
