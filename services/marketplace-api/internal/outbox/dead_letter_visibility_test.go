//go:build integration

package outbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// A dead-lettered row has to carry its REASON out to the console. `status`
// alone says the row was dead-lettered; without the reason the console cannot
// say why, and "dead-letter with a reason" (#260) is only half delivered —
// the same read-shipped-write-missing shape #405 exists to correct.
func TestIntegration_ListPlatform_CarriesDeadLetterReason(t *testing.T) {
	db := testdb.NewDB(t)
	tid := uuid.New()
	tenantID := tid.String()

	id := seedRow(t, db, tenantID, "product.created", time.Now().Add(-time.Hour), nil, nil)
	_, err := outbox.DeadLetterOne(context.Background(), db, id, "poison payload, will never deliver")
	require.NoError(t, err)

	res, err := outbox.ListPlatform(context.Background(), db,
		outbox.PlatformListFilter{TenantID: &tid}, time.Now())
	require.NoError(t, err)
	require.Len(t, res.Rows, 1)

	row := res.Rows[0]
	require.Equal(t, outbox.StatusDeadLettered, row.Status)
	require.NotNil(t, row.DeadLetteredAt, "the console needs to know WHEN it was dead-lettered")
	require.NotNil(t, row.DeadLetterReason, "the console needs to know WHY")
	require.Equal(t, "poison payload, will never deliver", *row.DeadLetterReason)
}

// The status filter and the derived status must agree about the same row.
// The CASE ranks published ABOVE dead_lettered, so a row carrying both
// markers reports "published"; the filter therefore must not return it under
// status=dead_lettered. Only reachable by hand-written SQL today — the write
// path refuses to dead-letter a published row — but the two definitions are
// written in different places and should not be allowed to drift.
func TestIntegration_ListPlatform_DeadLetteredFilterAgreesWithDerivedStatus(t *testing.T) {
	db := testdb.NewDB(t)
	tid := uuid.New()
	tenantID := tid.String()

	id := seedRow(t, db, tenantID, "product.created", time.Now().Add(-time.Hour), nil, nil)
	// Force the state the write path will not produce.
	require.NoError(t, db.Exec(
		`UPDATE outbox_events SET published_at = now(), dead_lettered_at = now(),
		        dead_letter_reason = 'set by hand' WHERE id = ?`, id).Error)

	byStatus, err := outbox.ListPlatform(context.Background(), db,
		outbox.PlatformListFilter{TenantID: &tid, Status: outbox.StatusDeadLettered}, time.Now())
	require.NoError(t, err)
	require.Empty(t, byStatus.Rows,
		"a row the CASE reports as published must not come back under status=dead_lettered")

	// And it is still visible as what it actually is.
	all, err := outbox.ListPlatform(context.Background(), db,
		outbox.PlatformListFilter{TenantID: &tid}, time.Now())
	require.NoError(t, err)
	require.Len(t, all.Rows, 1)
	require.Equal(t, outbox.StatusPublished, all.Rows[0].Status,
		"published still outranks dead_lettered in the derived status")
}
