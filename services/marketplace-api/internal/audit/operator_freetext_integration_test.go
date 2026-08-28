//go:build integration

package audit_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// #369 — the free-text `reason` on an operator row expires at 180 days; the
// row and its STRUCTURAL fields live the full OperatorRetentionYears.

// freeTextNow is the pinned clock every case below runs against, so cutoffs
// are exact rather than merely close (same discipline as the 7-year test).
var freeTextNow = time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)

// insertRowWithMetadata seeds one store-less audit row of the given actor
// type at a given age, carrying the supplied jsonb metadata.
func insertRowWithMetadata(t *testing.T, db *gorm.DB, actorType string, createdAt time.Time, metadata string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO audit_logs (id, tenant_id, store_id, actor_type, action, resource_type, status, severity, metadata, created_at)
		 VALUES (?, ?, NULL, ?, 'tenant.suspended', 'tenant', 'success', 'warning', ?::jsonb, ?)`,
		id, uuid.New(), actorType, metadata, createdAt,
	).Error)
	return id
}

// auditRow is the projection the assertions read back.
type auditRow struct {
	ActorType string
	Action    string
	Metadata  string
	CreatedAt time.Time
}

func readAuditRow(t *testing.T, db *gorm.DB, id uuid.UUID) auditRow {
	t.Helper()
	var got auditRow
	require.NoError(t, db.Raw(
		`SELECT actor_type, action, metadata::text AS metadata, created_at FROM audit_logs WHERE id = ?`, id,
	).Scan(&got).Error)
	return got
}

// metadataKey returns the value of a top-level metadata key, and whether it
// is present at all — the presence flag is what the strip is judged on.
func metadataKey(t *testing.T, db *gorm.DB, id uuid.UUID, key string) (string, bool) {
	t.Helper()
	var got struct {
		Present bool
		Val     *string
	}
	require.NoError(t, db.Raw(
		`SELECT jsonb_exists(metadata, ?) AS present, metadata->>? AS val FROM audit_logs WHERE id = ?`,
		key, key, id,
	).Scan(&got).Error)
	if got.Val == nil {
		return "", got.Present
	}
	return *got.Val, got.Present
}

func freeTextCutoff() time.Time {
	return freeTextNow.AddDate(0, 0, -audit.OperatorFreeTextRetentionDays)
}

func runPrune(t *testing.T, db *gorm.DB) audit.PruneStats {
	t.Helper()
	cron := audit.NewPruneCron(db, nil, func() time.Time { return freeTextNow }, 0)
	stats, err := cron.Run(context.Background())
	require.NoError(t, err)
	return stats
}

// 1. THE BOUNDARY, ON THE BOUNDARY. 180 days minus a second keeps the text.
func TestOperatorFreeText_JustInsideWindow_Kept(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs")

	// 179 days old. Stated as an ABSOLUTE age, not as cutoff±1s: an age
	// derived from OperatorFreeTextRetentionDays moves whenever the constant
	// moves, so it can only ever pin the comparison, never the window's
	// VALUE. This row is what fails if 180 is shortened.
	inside := insertRowWithMetadata(t, db, "operator", freeTextNow.AddDate(0, 0, -179),
		`{"reason_code":"fraud","reason":"jane@example.com disputing the chargeback, ticket 4471"}`)

	// ON the boundary: created_at == cutoff. The rule is created_at < cutoff
	// (strict), so a row exactly 180 days old has not yet EXCEEDED the
	// window and must keep its text. This is the only row that can
	// distinguish `<` from `<=`.
	onBoundary := insertRowWithMetadata(t, db, "operator", freeTextCutoff(),
		`{"reason_code":"fraud","reason":"still inside the window"}`)

	runPrune(t, db)

	val, present := metadataKey(t, db, inside, "reason")
	require.True(t, present, "a row one second inside 180 days must keep its free text")
	require.Equal(t, "jane@example.com disputing the chargeback, ticket 4471", val)

	_, present = metadataKey(t, db, onBoundary, "reason")
	require.True(t, present,
		"a row created EXACTLY 180 days ago must keep its free text: created_at < cutoff is strict, and equal is not less-than")
}

//  2. Past 180 days, `reason` is gone — but the ROW is still there and its
//     structural fields are intact. This is the whole point of splitting
//     retention by field rather than deleting the row early.
func TestOperatorFreeText_PastWindow_StrippedButRowAndStructureSurvive(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs")

	// 181 days old, stated absolutely for the same reason as case 1 — and
	// comfortably inside OperatorRetentionYears, so the 7-year delete pass
	// cannot be what removes the text here.
	createdAt := freeTextNow.AddDate(0, 0, -181)
	id := insertRowWithMetadata(t, db, "operator", createdAt,
		`{"reason_code":"fraud","reason":"jane@example.com disputing the chargeback, ticket 4471"}`)

	stats := runPrune(t, db)
	require.GreaterOrEqual(t, stats.OperatorFreeTextStripped, int64(1),
		"the strip pass must report the row it rewrote")

	require.Equal(t, int64(1), countRows(t, db, id),
		"the row itself lives OperatorRetentionYears; only the free text expires at 180 days")

	_, present := metadataKey(t, db, id, "reason")
	require.False(t, present, "free text one second past 180 days must be removed")

	code, codePresent := metadataKey(t, db, id, "reason_code")
	require.True(t, codePresent, "reason_code is structural and must survive the strip")
	require.Equal(t, "fraud", code)

	got := readAuditRow(t, db, id)
	require.Equal(t, "operator", got.ActorType, "actor_type must be untouched")
	require.Equal(t, "tenant.suspended", got.Action, "action must be untouched")
	require.WithinDuration(t, createdAt, got.CreatedAt, time.Millisecond,
		"created_at must be untouched — the strip is not a re-dating of history")
}

//  3. reason_code must NEVER be collateral damage — it is the field a
//     regulator's question turns on (#365 kept it deliberately).
func TestOperatorFreeText_ReasonCodeSurvives(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs")

	// A year old: well past the 180-day window so the strip definitely runs
	// over this row, and well inside the 7-year window so the row is still
	// there to be inspected.
	withBoth := insertRowWithMetadata(t, db, "operator", freeTextNow.AddDate(-1, 0, 0),
		`{"reason_code":"fraud","reason":"jane@example.com disputing the chargeback, ticket 4471"}`)
	// A row carrying ONLY the code: nothing to strip, and the code stays.
	codeOnly := insertRowWithMetadata(t, db, "operator", freeTextNow.AddDate(-1, 0, 0),
		`{"reason_code":"policy_violation"}`)

	runPrune(t, db)

	code, present := metadataKey(t, db, withBoth, "reason_code")
	require.True(t, present, "stripping `reason` must never take `reason_code` with it")
	require.Equal(t, "fraud", code, "reason_code must keep its original value")

	code, present = metadataKey(t, db, codeOnly, "reason_code")
	require.True(t, present, "a row with no free text must be left entirely alone")
	require.Equal(t, "policy_violation", code)
}

// 4. Non-operator rows are untouched, however old.
func TestOperatorFreeText_NonOperatorRowUnaffected(t *testing.T) {
	db := testdb.NewDB(t, "audit_logs")

	// A year old — far past 180 days — but not an operator row. The purge
	// deletes these on erasure, so their free text is not #369's problem,
	// and the strip must not reach them.
	id := insertRowWithMetadata(t, db, "system", freeTextNow.AddDate(-1, 0, 0),
		`{"reason_code":"fraud","reason":"jane@example.com disputing the chargeback, ticket 4471"}`)

	runPrune(t, db)

	require.Equal(t, int64(1), countRows(t, db, id), "non-operator rows are not deleted by this cron")

	val, present := metadataKey(t, db, id, "reason")
	require.True(t, present, "the 180-day strip applies to actor_type='operator' only")
	require.Equal(t, "jane@example.com disputing the chargeback, ticket 4471", val)
}
