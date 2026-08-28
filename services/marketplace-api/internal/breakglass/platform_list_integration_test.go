//go:build integration

package breakglass_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mark8ly/marketplace-api/internal/breakglass"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedAccount inserts a minimal break_glass_accounts row for tenantID,
// satisfying every NOT NULL column (secret_path, password_hash,
// totp_secret_ref) with harmless test placeholders — none of these three
// columns should ever be reachable through ListPlatform, which is exactly
// what TestIntegration_ListPlatform_SelectsExplicitColumnsNeverCredentialFields
// guards.
func seedAccount(t *testing.T, db *gorm.DB, tenantID uuid.UUID, lastUsedAt *time.Time) {
	t.Helper()
	require.NoError(t, db.Exec(
		`INSERT INTO break_glass_accounts
			(tenant_id, secret_path, password_hash, totp_secret_ref, totp_enrolled, last_used_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		tenantID,
		"/projects/tesserix-test/secrets/break-glass-"+tenantID.String(),
		"$2a$12$test-hash-not-a-real-bcrypt-value",
		"$.totp_secret",
		false,
		lastUsedAt,
	).Error)
}

// seedLockout inserts a break_glass_lockouts row. tenantID may be nil,
// matching an attempt that never resolved a tenant. ip_hash is random per
// call so repeated inserts in one test never collide on the
// (ip_hash, locked_until) primary key.
//
// Uses the Create(map) builder rather than a raw Exec with a `VALUES (?, ...)`
// string: GORM's raw-SQL placeholder expansion treats any slice/array
// argument immediately inside parentheses as an IN-list and explodes a
// []byte ip_hash into one bound value per byte, which Postgres then rejects
// as "INSERT has more expressions than target columns".
func seedLockout(t *testing.T, db *gorm.DB, tenantID *uuid.UUID, lockedUntil time.Time, reason string) {
	t.Helper()
	ipHash := uuid.New()
	require.NoError(t, db.Table("break_glass_lockouts").Create(map[string]interface{}{
		"ip_hash":      ipHash[:],
		"tenant_id":    tenantID,
		"locked_until": lockedUntil,
		"reason":       reason,
	}).Error)
}

func indexByTenant(rows []breakglass.PlatformRow) map[uuid.UUID]int {
	m := make(map[uuid.UUID]int, len(rows))
	for i, r := range rows {
		m[r.TenantID] = i
	}
	return m
}

// The endpoint exists to answer "which emergency accounts have been used
// recently". A never-used account floating to the top under the default
// sort would defeat that. sqlite and Postgres disagree on default NULL
// ordering, which is exactly why this needs a real-Postgres assertion.
func TestIntegration_ListPlatform_NeverUsedSortsLastUnderDefaultDescOrder(t *testing.T) {
	db := testdb.NewTx(t)
	now := time.Now().UTC().Truncate(time.Second)

	recentUsed, olderUsed, neverUsed := uuid.New(), uuid.New(), uuid.New()
	used2h := now.Add(-2 * time.Hour)
	used30d := now.Add(-30 * 24 * time.Hour)

	seedAccount(t, db, recentUsed, &used2h)
	seedAccount(t, db, olderUsed, &used30d)
	seedAccount(t, db, neverUsed, nil)

	res, err := breakglass.ListPlatform(context.Background(), db,
		breakglass.PlatformListFilter{SortDesc: true}, now)
	require.NoError(t, err)

	idx := indexByTenant(res.Accounts)
	posRecent, ok1 := idx[recentUsed]
	posOlder, ok2 := idx[olderUsed]
	posNever, ok3 := idx[neverUsed]
	require.True(t, ok1 && ok2 && ok3, "all three seeded accounts must appear in the page")

	require.Less(t, posRecent, posOlder,
		"a more recently used account must sort before an older one under DESC")
	require.Less(t, posOlder, posNever,
		"a never-used account must sort LAST under DESC NULLS LAST, not first")
}

// Sibling to TestIntegration_ListPlatform_NeverUsedSortsLastUnderDefaultDescOrder:
// that test proves NULLS LAST holds under the default DESC order
// (order.go's "a.last_used_at DESC NULLS LAST, a.tenant_id" branch). This
// proves the OTHER branch — "a.last_used_at ASC NULLS LAST, a.tenant_id",
// reachable only via ?sort=last_used_at (SortDesc: false) — is reachable
// and still puts the never-used account last. Someone who "simplified"
// that branch to plain ASC (dropping NULLS LAST) would regress ordering
// only in Postgres, not sqlite, and only under this one query parameter —
// exactly what this asserts against.
func TestIntegration_ListPlatform_NeverUsedSortsLastUnderAscOrderToo(t *testing.T) {
	db := testdb.NewTx(t)
	now := time.Now().UTC().Truncate(time.Second)

	older30d, recent2h, neverUsed := uuid.New(), uuid.New(), uuid.New()
	used30d := now.Add(-30 * 24 * time.Hour)
	used2h := now.Add(-2 * time.Hour)

	seedAccount(t, db, older30d, &used30d)
	seedAccount(t, db, recent2h, &used2h)
	seedAccount(t, db, neverUsed, nil)

	res, err := breakglass.ListPlatform(context.Background(), db,
		breakglass.PlatformListFilter{SortDesc: false}, now)
	require.NoError(t, err)

	idx := indexByTenant(res.Accounts)
	posOlder, ok1 := idx[older30d]
	posRecent, ok2 := idx[recent2h]
	posNever, ok3 := idx[neverUsed]
	require.True(t, ok1 && ok2 && ok3, "all three seeded accounts must appear in the page")

	require.Less(t, posOlder, posRecent,
		"an older-used account must sort before a more recently used one under ASC")
	require.Less(t, posRecent, posNever,
		"a never-used account must sort LAST under ASC NULLS LAST too, not first")
}

// break_glass_lockouts is keyed (ip_hash, locked_until) with a nullable
// tenant_id: "locked_out" means "at least one active lockout row currently
// names this tenant", not "this account is locked" in any account-level
// sense. This test seeds all three shapes the plan calls out.
func TestIntegration_ListPlatform_LockoutVisibilitySemantics(t *testing.T) {
	db := testdb.NewTx(t)
	now := time.Now().UTC().Truncate(time.Second)

	tenantB, tenantC, tenantD := uuid.New(), uuid.New(), uuid.New()
	seedAccount(t, db, tenantB, nil)
	seedAccount(t, db, tenantC, nil)
	seedAccount(t, db, tenantD, nil)

	activeExpiry := now.Add(1 * time.Hour)
	seedLockout(t, db, &tenantB, activeExpiry, "3-strike")                           // active, names B
	seedLockout(t, db, &tenantC, now.Add(-1*time.Hour), "3-strike")                  // expired, names C
	seedLockout(t, db, nil, now.Add(1*time.Hour), "attempt never resolved a tenant") // active, tenant_id IS NULL

	resB, err := breakglass.ListPlatform(context.Background(), db,
		breakglass.PlatformListFilter{TenantID: &tenantB}, now)
	require.NoError(t, err)
	require.Len(t, resB.Accounts, 1)
	require.True(t, resB.Accounts[0].LockedOut,
		"an active lockout naming this tenant must show as locked out")
	require.NotNil(t, resB.Accounts[0].LockoutExpiresAt)
	require.WithinDuration(t, activeExpiry, *resB.Accounts[0].LockoutExpiresAt, time.Second)

	resC, err := breakglass.ListPlatform(context.Background(), db,
		breakglass.PlatformListFilter{TenantID: &tenantC}, now)
	require.NoError(t, err)
	require.Len(t, resC.Accounts, 1)
	require.False(t, resC.Accounts[0].LockedOut, "an EXPIRED lockout must not count as active")
	require.Nil(t, resC.Accounts[0].LockoutExpiresAt)

	resD, err := breakglass.ListPlatform(context.Background(), db,
		breakglass.PlatformListFilter{TenantID: &tenantD}, now)
	require.NoError(t, err)
	require.Len(t, resD.Accounts, 1)
	require.False(t, resD.Accounts[0].LockedOut,
		"a lockout row with tenant_id IS NULL must not attach to any account")
	require.Nil(t, resD.Accounts[0].LockoutExpiresAt)
}

// TestIntegration_ListPlatform_LockedFilterMatchesTheNotExistsInversion
// covers PlatformListFilter.Locked at the data layer: only the
// handler→filter plumbing (parseBreakGlassBool wiring "locked" into
// f.Locked) had coverage before this, never the NOT EXISTS inversion
// Locked=false compiles down to in ListPlatform. Reuses the same lockout
// fixture shapes as TestIntegration_ListPlatform_LockoutVisibilitySemantics
// above: an active lockout naming a tenant, and an EXPIRED lockout naming
// a different tenant.
func TestIntegration_ListPlatform_LockedFilterMatchesTheNotExistsInversion(t *testing.T) {
	db := testdb.NewTx(t)
	now := time.Now().UTC().Truncate(time.Second)

	activelyLocked, expiredOnly := uuid.New(), uuid.New()
	seedAccount(t, db, activelyLocked, nil)
	seedAccount(t, db, expiredOnly, nil)

	seedLockout(t, db, &activelyLocked, now.Add(1*time.Hour), "3-strike") // active
	seedLockout(t, db, &expiredOnly, now.Add(-1*time.Hour), "3-strike")   // expired

	trueVal, falseVal := true, false

	resLocked, err := breakglass.ListPlatform(context.Background(), db,
		breakglass.PlatformListFilter{Locked: &trueVal}, now)
	require.NoError(t, err)
	idxLocked := indexByTenant(resLocked.Accounts)
	_, hasActive := idxLocked[activelyLocked]
	_, hasExpired := idxLocked[expiredOnly]
	require.True(t, hasActive,
		"Locked:true must include the tenant with an active lockout")
	require.False(t, hasExpired,
		"Locked:true must exclude the tenant whose only lockout is EXPIRED — "+
			"an expired lockout must behave as unlocked")

	resUnlocked, err := breakglass.ListPlatform(context.Background(), db,
		breakglass.PlatformListFilter{Locked: &falseVal}, now)
	require.NoError(t, err)
	idxUnlocked := indexByTenant(resUnlocked.Accounts)
	_, hasActiveUnlocked := idxUnlocked[activelyLocked]
	_, hasExpiredUnlocked := idxUnlocked[expiredOnly]
	require.False(t, hasActiveUnlocked,
		"Locked:false (the NOT EXISTS inversion) must exclude the actively "+
			"locked tenant")
	require.True(t, hasExpiredUnlocked,
		"Locked:false must include the tenant whose only lockout is "+
			"EXPIRED — it must behave as unlocked in BOTH directions")
}

func TestIntegration_ListPlatform_UsedAfterAndUsedBeforeNarrow(t *testing.T) {
	db := testdb.NewTx(t)
	now := time.Now().UTC().Truncate(time.Second)

	early, mid, late, never := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	tEarly := now.Add(-3 * time.Hour)
	tMid := now.Add(-90 * time.Minute)
	tLate := now.Add(-10 * time.Minute)

	seedAccount(t, db, early, &tEarly)
	seedAccount(t, db, mid, &tMid)
	seedAccount(t, db, late, &tLate)
	seedAccount(t, db, never, nil)

	after := now.Add(-2 * time.Hour)
	res, err := breakglass.ListPlatform(context.Background(), db,
		breakglass.PlatformListFilter{UsedAfter: &after}, now)
	require.NoError(t, err)
	idx := indexByTenant(res.Accounts)
	_, hasMid := idx[mid]
	_, hasLate := idx[late]
	_, hasEarly := idx[early]
	_, hasNever := idx[never]
	require.True(t, hasMid && hasLate, "rows used after the bound must be included")
	require.False(t, hasEarly, "a row used before the bound must be excluded")
	require.False(t, hasNever, "a never-used row must not match used_after")

	before := now.Add(-1 * time.Hour)
	res, err = breakglass.ListPlatform(context.Background(), db,
		breakglass.PlatformListFilter{UsedBefore: &before}, now)
	require.NoError(t, err)
	idx = indexByTenant(res.Accounts)
	_, hasEarly = idx[early]
	_, hasMid = idx[mid]
	_, hasLate = idx[late]
	_, hasNever = idx[never]
	require.True(t, hasEarly && hasMid, "rows used before the bound must be included")
	require.False(t, hasLate, "a row used after the bound must be excluded")
	require.False(t, hasNever, "a never-used row must not match used_before")
}

// Total is the unpaginated match count while the returned page is limited —
// the property that makes total/limit a correct page count.
func TestIntegration_ListPlatform_TotalIsUnpaginatedCount(t *testing.T) {
	db := testdb.NewTx(t)
	now := time.Now().UTC()

	for i := 0; i < 5; i++ {
		used := now.Add(-time.Duration(i) * time.Hour)
		seedAccount(t, db, uuid.New(), &used)
	}

	res, err := breakglass.ListPlatform(context.Background(), db,
		breakglass.PlatformListFilter{Limit: 2}, now)
	require.NoError(t, err)
	require.EqualValues(t, 5, res.Total, "Total must be the full match count, not the page size")
	require.Len(t, res.Accounts, 2, "the returned page must respect Limit")
}

// sqlCapture is a minimal gorm logger.Interface that records every SQL
// statement traced through it, so the test can inspect the exact query
// ListPlatform sends to Postgres.
type sqlCapture struct {
	queries *[]string
}

func (c *sqlCapture) LogMode(logger.LogLevel) logger.Interface      { return c }
func (c *sqlCapture) Info(context.Context, string, ...interface{})  {}
func (c *sqlCapture) Warn(context.Context, string, ...interface{})  {}
func (c *sqlCapture) Error(context.Context, string, ...interface{}) {}
func (c *sqlCapture) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	*c.queries = append(*c.queries, sql)
}

func findPageQuery(t *testing.T, queries []string) string {
	t.Helper()
	for _, q := range queries {
		if strings.Contains(q, "lockout_expires_at") {
			return q
		}
	}
	t.Fatalf("no captured query looked like the row-fetch select; captured: %v", queries)
	return ""
}

// This is the security property of the whole change: break_glass_accounts
// holds a GCP Secret Manager path pointing at the live plaintext password
// and TOTP secret, a bcrypt hash, and a JSON pointer into that blob. None of
// them may be reachable from this cross-tenant platform read.
func TestIntegration_ListPlatform_SelectsExplicitColumnsNeverCredentialFields(t *testing.T) {
	db := testdb.NewTx(t)
	now := time.Now().UTC()
	tid := uuid.New()
	seedAccount(t, db, tid, nil)

	var captured []string
	capDB := db.Session(&gorm.Session{Logger: &sqlCapture{queries: &captured}})

	_, err := breakglass.ListPlatform(context.Background(), capDB,
		breakglass.PlatformListFilter{TenantID: &tid}, now)
	require.NoError(t, err)

	pageQuery := findPageQuery(t, captured)
	t.Logf("captured page query: %s", pageQuery)

	for _, forbidden := range []string{"*", "secret_path", "password_hash", "totp_secret_ref"} {
		require.NotContains(t, pageQuery, forbidden,
			"the row-fetch query must never be able to reach %q", forbidden)
	}
}
