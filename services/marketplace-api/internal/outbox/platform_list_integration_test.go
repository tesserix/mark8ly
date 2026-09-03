//go:build integration

package outbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// listAsOf is a fixed instant so age assertions are exact rather than
// approximate. Every fixture below is placed relative to it.
var listAsOf = time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)

// seedRow inserts one outbox_events row in a chosen state. published and
// errMsg are independent so the three legacy states can be built explicitly
// rather than inferred. Delegates to seedRowFull with no dead-letter
// columns.
func seedRow(t *testing.T, db *gorm.DB, tenantID string, eventType string,
	createdAt time.Time, published *time.Time, errMsg *string) string {
	t.Helper()
	return seedRowFull(t, db, tenantID, eventType, createdAt, published, errMsg, nil, nil)
}

// seedRowFull is seedRow plus the two dead-letter columns (#405), so a
// fourth, dead_lettered state can be built explicitly too.
func seedRowFull(t *testing.T, db *gorm.DB, tenantID string, eventType string,
	createdAt time.Time, published *time.Time, errMsg *string,
	deadLetteredAt *time.Time, deadLetterReason *string) string {
	t.Helper()
	id := uuid.NewString()
	err := db.Exec(`
		INSERT INTO outbox_events
			(id, tenant_id, aggregate, aggregate_id, event_type, payload, created_at, published_at, error,
			 dead_lettered_at, dead_letter_reason)
		VALUES (?, ?, 'product', ?, ?, '{"store_id":"11111111-1111-1111-1111-111111111111","secret":"do-not-leak"}'::jsonb,
			?, ?, ?, ?, ?)`,
		id, tenantID, uuid.NewString(), eventType, createdAt, published, errMsg,
		deadLetteredAt, deadLetterReason).Error
	require.NoError(t, err)
	return id
}

func TestIntegration_ListPlatform_DerivesAllThreeStates(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	tenantID := uuid.NewString()
	pubAt := listAsOf.Add(-30 * time.Minute)
	failReason := outbox.ReasonPayloadUnparseable

	pendingID := seedRow(t, db, tenantID, "product.created", listAsOf.Add(-10*time.Minute), nil, nil)
	failedID := seedRow(t, db, tenantID, "product.updated", listAsOf.Add(-20*time.Minute), nil, &failReason)
	publishedID := seedRow(t, db, tenantID, "product.deleted", listAsOf.Add(-40*time.Minute), &pubAt, nil)

	got, err := outbox.ListPlatform(context.Background(), db, outbox.PlatformListFilter{}, listAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(3), got.Total)
	require.Len(t, got.Rows, 3)

	byID := map[string]outbox.PlatformRow{}
	for _, r := range got.Rows {
		byID[r.ID] = r
	}

	require.Equal(t, outbox.StatusPending, byID[pendingID].Status)
	require.Equal(t, outbox.StatusFailed, byID[failedID].Status)
	require.Equal(t, outbox.StatusPublished, byID[publishedID].Status)

	// Age is present and EXACT for unpublished rows.
	require.NotNil(t, byID[pendingID].AgeSeconds)
	require.Equal(t, int64(600), *byID[pendingID].AgeSeconds)
	require.NotNil(t, byID[failedID].AgeSeconds)
	require.Equal(t, int64(1200), *byID[failedID].AgeSeconds)

	// Absent for a published row: a settled row has no waiting time, and a
	// growing number there would read as "stuck" beside a genuinely stuck row.
	require.Nil(t, byID[publishedID].AgeSeconds,
		"a published row must have no age_seconds")

	// The failure reason is carried through verbatim.
	require.NotNil(t, byID[failedID].Error)
	require.Equal(t, outbox.ReasonPayloadUnparseable, *byID[failedID].Error)
	require.Nil(t, byID[pendingID].Error)
}

// ListPlatform derives all FOUR states (#405 adds dead_lettered to the
// pending/failed/published set), and published still wins over BOTH error
// and dead_lettered_at — the precedence pinned by
// TestIntegration_ListPlatform_PublishedWinsOverError must not regress when
// a row also carries a dead-letter mark.
func TestIntegration_ListPlatform_DerivesAllFourStates(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	tenantID := uuid.NewString()
	pubAt := listAsOf.Add(-30 * time.Minute)
	failReason := outbox.ReasonPayloadUnparseable
	deadAt := listAsOf.Add(-5 * time.Minute)
	deadReason := "manual duplicate"

	pendingID := seedRow(t, db, tenantID, "product.created", listAsOf.Add(-10*time.Minute), nil, nil)
	failedID := seedRow(t, db, tenantID, "product.updated", listAsOf.Add(-20*time.Minute), nil, &failReason)
	publishedID := seedRow(t, db, tenantID, "product.deleted", listAsOf.Add(-40*time.Minute), &pubAt, nil)
	deadLetteredID := seedRowFull(t, db, tenantID, "order.placed", listAsOf.Add(-15*time.Minute), nil, nil, &deadAt, &deadReason)
	// published_at AND both error and dead_lettered_at set: published must
	// still win over both.
	publishedButMarkedID := seedRowFull(t, db, tenantID, "order.cancelled", listAsOf.Add(-50*time.Minute), &pubAt, &failReason, &deadAt, &deadReason)

	got, err := outbox.ListPlatform(context.Background(), db, outbox.PlatformListFilter{}, listAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(5), got.Total)

	byID := map[string]outbox.PlatformRow{}
	for _, r := range got.Rows {
		byID[r.ID] = r
	}

	require.Equal(t, outbox.StatusPending, byID[pendingID].Status)
	require.Equal(t, outbox.StatusFailed, byID[failedID].Status)
	require.Equal(t, outbox.StatusPublished, byID[publishedID].Status)
	require.Equal(t, outbox.StatusDeadLettered, byID[deadLetteredID].Status)
	require.Equal(t, outbox.StatusPublished, byID[publishedButMarkedID].Status,
		"published_at wins over both error and dead_lettered_at")
}

func TestIntegration_ListPlatform_StatusFilterNarrows(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	tenantID := uuid.NewString()
	pubAt := listAsOf.Add(-30 * time.Minute)
	failReason := outbox.ReasonStoreNotFound
	deadAt := listAsOf.Add(-15 * time.Minute)
	deadReason := "confirmed duplicate of a manually-corrected order"

	seedRow(t, db, tenantID, "product.created", listAsOf.Add(-10*time.Minute), nil, nil)
	seedRow(t, db, tenantID, "product.updated", listAsOf.Add(-20*time.Minute), nil, &failReason)
	seedRow(t, db, tenantID, "product.deleted", listAsOf.Add(-40*time.Minute), &pubAt, nil)
	seedRowFull(t, db, tenantID, "order.placed", listAsOf.Add(-25*time.Minute), nil, nil, &deadAt, &deadReason)

	for _, tc := range []struct {
		status string
		want   int
	}{
		{outbox.StatusPending, 1},
		{outbox.StatusFailed, 1},
		{outbox.StatusPublished, 1},
		{outbox.StatusDeadLettered, 1},
	} {
		got, err := outbox.ListPlatform(context.Background(), db,
			outbox.PlatformListFilter{Status: tc.status}, listAsOf)
		require.NoError(t, err, tc.status)
		require.Equal(t, int64(tc.want), got.Total, "total for status=%s", tc.status)
		require.Len(t, got.Rows, tc.want, "rows for status=%s", tc.status)
		require.Equal(t, tc.status, got.Rows[0].Status)
	}

	// An unrecognised status narrows NOTHING rather than erroring or
	// returning empty — the established contract on this surface.
	got, err := outbox.ListPlatform(context.Background(), db,
		outbox.PlatformListFilter{Status: "banana"}, listAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(4), got.Total, "an unknown status must narrow nothing")

}

// older_than_minutes answers "what is stuck", so it applies to UNPUBLISHED
// rows only. A published row is settled however old it is.
func TestIntegration_ListPlatform_OlderThanMinutesExcludesPublished(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	tenantID := uuid.NewString()
	oldPub := listAsOf.Add(-30 * time.Minute)

	oldPendingID := seedRow(t, db, tenantID, "product.created", listAsOf.Add(-60*time.Minute), nil, nil)
	seedRow(t, db, tenantID, "product.updated", listAsOf.Add(-1*time.Minute), nil, nil)       // young pending
	seedRow(t, db, tenantID, "product.deleted", listAsOf.Add(-600*time.Minute), &oldPub, nil) // very old, published

	got, err := outbox.ListPlatform(context.Background(), db,
		outbox.PlatformListFilter{OlderThanMinutes: 30}, listAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Total)
	require.Len(t, got.Rows, 1)
	require.Equal(t, oldPendingID, got.Rows[0].ID,
		"only the old UNPUBLISHED row may match; the older published row is settled")
}

func TestIntegration_ListPlatform_TenantAndEventTypeNarrow(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	tenantA := uuid.NewString()
	tenantB := uuid.NewString()

	wantID := seedRow(t, db, tenantA, "product.created", listAsOf.Add(-5*time.Minute), nil, nil)
	seedRow(t, db, tenantA, "product.updated", listAsOf.Add(-5*time.Minute), nil, nil)
	seedRow(t, db, tenantB, "product.created", listAsOf.Add(-5*time.Minute), nil, nil)

	tid := uuid.MustParse(tenantA)
	got, err := outbox.ListPlatform(context.Background(), db,
		outbox.PlatformListFilter{TenantID: &tid, EventType: "product.created"}, listAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Total)
	require.Equal(t, wantID, got.Rows[0].ID)
}

func TestIntegration_ListPlatform_ClampsLimitAndPages(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	tenantID := uuid.NewString()
	for i := 0; i < 3; i++ {
		seedRow(t, db, tenantID, "product.created",
			listAsOf.Add(-time.Duration(i+1)*time.Minute), nil, nil)
	}

	// An oversized limit CLAMPS rather than refusing.
	got, err := outbox.ListPlatform(context.Background(), db,
		outbox.PlatformListFilter{Limit: 100000}, listAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(3), got.Total)
	require.Len(t, got.Rows, 3)

	// Page 2 of 2-per-page returns the remaining row, and Total stays the
	// FULL count, not the page size.
	got, err = outbox.ListPlatform(context.Background(), db,
		outbox.PlatformListFilter{Limit: 2, Page: 2}, listAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(3), got.Total)
	require.Len(t, got.Rows, 1)
}

func TestIntegration_ListPlatform_EmptyIsNotAnError(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	got, err := outbox.ListPlatform(context.Background(), db, outbox.PlatformListFilter{}, listAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(0), got.Total)
	require.Empty(t, got.Rows)
}

// The unfiltered read must span every tenant. This is the whole reason #331
// exists: the console asks estate-wide questions. TenantAndEventTypeNarrow
// proves a filter EXCLUDES other tenants; only this proves the absence of a
// filter INCLUDES them. Without it, a refactor that accidentally
// tenant-scoped the base query would leave every other test green.
func TestIntegration_ListPlatform_IsCrossTenantByDefault(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	tenantA := uuid.NewString()
	tenantB := uuid.NewString()
	tenantC := uuid.NewString()

	seedRow(t, db, tenantA, "product.created", listAsOf.Add(-5*time.Minute), nil, nil)
	seedRow(t, db, tenantB, "product.updated", listAsOf.Add(-6*time.Minute), nil, nil)
	seedRow(t, db, tenantC, "order.placed", listAsOf.Add(-7*time.Minute), nil, nil)

	got, err := outbox.ListPlatform(context.Background(), db, outbox.PlatformListFilter{}, listAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(3), got.Total)

	seen := map[string]bool{}
	for _, r := range got.Rows {
		seen[r.TenantID] = true
	}
	require.Len(t, seen, 3, "an unfiltered list must span every tenant")
	require.True(t, seen[tenantA])
	require.True(t, seen[tenantB])
	require.True(t, seen[tenantC])
}

// A row carrying BOTH published_at and error. The publisher writes one or
// the other and never both, so this is unreachable in-service — but the
// documented operator requeue path is a manual UPDATE, so a human can
// produce it. published must win: the row WAS delivered, whatever stale
// error it still carries.
func TestIntegration_ListPlatform_PublishedWinsOverError(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	tenantID := uuid.NewString()
	pubAt := listAsOf.Add(-10 * time.Minute)
	stale := outbox.ReasonPayloadUnparseable

	id := seedRow(t, db, tenantID, "product.created", listAsOf.Add(-30*time.Minute), &pubAt, &stale)

	got, err := outbox.ListPlatform(context.Background(), db, outbox.PlatformListFilter{}, listAsOf)
	require.NoError(t, err)
	require.Len(t, got.Rows, 1)
	require.Equal(t, id, got.Rows[0].ID)
	require.Equal(t, outbox.StatusPublished, got.Rows[0].Status,
		"published_at is tested before error: a delivered row is published whatever error it carries")
	// The two CASE expressions must agree with each other, not just each be
	// individually right.
	require.Nil(t, got.Rows[0].AgeSeconds,
		"a row classified published must carry no age_seconds")
}

func TestIntegration_ListPlatform_PublishedPlusOlderThanIsDeliberatelyEmpty(t *testing.T) {
	db := testdb.NewDB(t, "outbox_events")
	tenantID := uuid.NewString()
	pubAt := listAsOf.Add(-10 * time.Minute)
	seedRow(t, db, tenantID, "product.created", listAsOf.Add(-600*time.Minute), &pubAt, nil)

	got, err := outbox.ListPlatform(context.Background(), db,
		outbox.PlatformListFilter{Status: outbox.StatusPublished, OlderThanMinutes: 30}, listAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(0), got.Total,
		"older_than_minutes narrows to UNPUBLISHED rows, so pairing it with status=published is a "+
			"contradiction that returns nothing — deliberate, not a bug")

	// Symmetric contradiction on the fourth state (#405): a dead-lettered
	// row is terminal, not stuck, so older_than_minutes excludes it via its
	// own "AND dead_lettered_at IS NULL" term — pairing it with
	// status=dead_lettered is the same kind of deliberate-empty combination,
	// not a bug to investigate.
	deadAt := listAsOf.Add(-10 * time.Minute)
	deadReason := "duplicate"
	seedRowFull(t, db, tenantID, "product.updated", listAsOf.Add(-600*time.Minute), nil, nil, &deadAt, &deadReason)

	got, err = outbox.ListPlatform(context.Background(), db,
		outbox.PlatformListFilter{Status: outbox.StatusDeadLettered, OlderThanMinutes: 30}, listAsOf)
	require.NoError(t, err)
	require.Equal(t, int64(0), got.Total,
		"older_than_minutes narrows to rows with dead_lettered_at IS NULL, so pairing it with "+
			"status=dead_lettered is also a deliberate-empty contradiction")
}
