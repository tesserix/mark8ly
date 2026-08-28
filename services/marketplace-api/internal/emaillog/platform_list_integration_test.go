//go:build integration

package emaillog_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/emaillog"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func seedRow(t *testing.T, db *gorm.DB, tenantID *uuid.UUID, kind, status string, createdAt time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO email_sends (id, tenant_id, recipient, kind, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, tenantID, uuid.NewString()+"@example.com", kind, status, createdAt).Error)
	return id
}

func TestIntegration_PlatformList_FiltersByTenantKindAndStatus(t *testing.T) {
	db := testdb.NewTx(t)
	mine, theirs := uuid.New(), uuid.New()
	now := time.Now().UTC()

	seedRow(t, db, &mine, "giftcard", emaillog.StatusDelivered, now)
	seedRow(t, db, &mine, "ticket", emaillog.StatusBounced, now)
	seedRow(t, db, &theirs, "giftcard", emaillog.StatusDelivered, now)

	res, err := emaillog.ListPlatform(context.Background(), db,
		emaillog.PlatformListFilter{TenantID: &mine}, now)
	require.NoError(t, err)
	require.EqualValues(t, 2, res.Total, "another tenant's sends must not leak in")

	res, err = emaillog.ListPlatform(context.Background(), db,
		emaillog.PlatformListFilter{TenantID: &mine, Kind: "giftcard"}, now)
	require.NoError(t, err)
	require.EqualValues(t, 1, res.Total)

	res, err = emaillog.ListPlatform(context.Background(), db,
		emaillog.PlatformListFilter{TenantID: &mine, Status: emaillog.StatusBounced}, now)
	require.NoError(t, err)
	require.EqualValues(t, 1, res.Total)
	require.Equal(t, "ticket", res.Sends[0].Kind)
}

// An unrecognised status narrows nothing, matching every other unknown
// parameter on this surface. Silently returning zero rows would read as "no
// such mail" rather than "no such filter".
func TestIntegration_PlatformList_UnknownStatusNarrowsNothing(t *testing.T) {
	db := testdb.NewTx(t)
	tid := uuid.New()
	now := time.Now().UTC()
	seedRow(t, db, &tid, "ticket", emaillog.StatusSent, now)

	res, err := emaillog.ListPlatform(context.Background(), db,
		emaillog.PlatformListFilter{TenantID: &tid, Status: "not-a-status"}, now)
	require.NoError(t, err)
	require.EqualValues(t, 1, res.Total)
}

// The question this endpoint exists to answer: what never completed.
func TestIntegration_PlatformList_StuckFindsOnlyUnfinishedRows(t *testing.T) {
	db := testdb.NewTx(t)
	tid := uuid.New()
	now := time.Now().UTC()

	stuck := seedRow(t, db, &tid, "orderdoc", emaillog.StatusSending, now.Add(-90*time.Minute))
	seedRow(t, db, &tid, "orderdoc", emaillog.StatusSending, now.Add(-1*time.Minute))
	// Old but settled: age does not make a delivered send interesting.
	seedRow(t, db, &tid, "orderdoc", emaillog.StatusDelivered, now.Add(-10*time.Hour))

	res, err := emaillog.ListPlatform(context.Background(), db,
		emaillog.PlatformListFilter{TenantID: &tid, StuckMinutes: 60}, now)
	require.NoError(t, err)
	require.EqualValues(t, 1, res.Total)
	require.Equal(t, stuck, res.Sends[0].ID)

	require.NotNil(t, res.Sends[0].AgeSeconds, "a stuck row reports how long it has been stuck")
	require.Greater(t, *res.Sends[0].AgeSeconds, int64(3000))
}

// age_seconds is NULL for anything settled — by the same CASE that makes it
// settled. One definition of "stuck" in the estate.
func TestIntegration_PlatformList_AgeIsNullForSettledRows(t *testing.T) {
	db := testdb.NewTx(t)
	tid := uuid.New()
	now := time.Now().UTC()
	seedRow(t, db, &tid, "ticket", emaillog.StatusDelivered, now.Add(-5*time.Hour))

	res, err := emaillog.ListPlatform(context.Background(), db,
		emaillog.PlatformListFilter{TenantID: &tid}, now)
	require.NoError(t, err)
	require.Len(t, res.Sends, 1)
	require.Nil(t, res.Sends[0].AgeSeconds)
}

// Total is the full match count, not the page size — the property that makes
// total/limit a correct page count.
func TestIntegration_PlatformList_TotalIsUnpaginated(t *testing.T) {
	db := testdb.NewTx(t)
	tid := uuid.New()
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		seedRow(t, db, &tid, "ticket", emaillog.StatusSent, now.Add(-time.Duration(i)*time.Minute))
	}

	res, err := emaillog.ListPlatform(context.Background(), db,
		emaillog.PlatformListFilter{TenantID: &tid, Limit: 2}, now)
	require.NoError(t, err)
	require.EqualValues(t, 5, res.Total)
	require.Len(t, res.Sends, 2)
}

// Empty is an allocated slice, never nil — a nil slice marshals to null and
// defeats a caller's `?? []`.
func TestIntegration_PlatformList_EmptyIsAllocated(t *testing.T) {
	db := testdb.NewTx(t)
	none := uuid.New()
	res, err := emaillog.ListPlatform(context.Background(), db,
		emaillog.PlatformListFilter{TenantID: &none}, time.Now().UTC())
	require.NoError(t, err)
	require.NotNil(t, res.Sends)
	require.Empty(t, res.Sends)
}
