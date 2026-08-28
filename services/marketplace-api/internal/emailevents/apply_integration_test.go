//go:build integration

package emailevents_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/emailevents"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func seedSend(t *testing.T, db *gorm.DB, status string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO email_sends (id, recipient, kind, status, created_at)
		 VALUES (?, ?, 'giftcard', ?, now())`,
		id, uuid.NewString()+"@example.com", status).Error)
	return id
}

func statusOf(t *testing.T, db *gorm.DB, id uuid.UUID) string {
	t.Helper()
	var s string
	require.NoError(t, db.Raw(`SELECT status FROM email_sends WHERE id = ?`, id).Scan(&s).Error)
	return s
}

func TestIntegration_Apply_DeliveredAdvancesTheRow(t *testing.T) {
	db := testdb.NewTx(t)
	id := seedSend(t, db, "sent")
	a := emailevents.NewApplier(db, nil)

	require.NoError(t, a.Apply(context.Background(), emailevents.Event{
		EventID: "evt_1", SendID: id, Type: "email.delivered", At: time.Now(),
	}))
	require.Equal(t, "delivered", statusOf(t, db, id))
}

// Webhooks are at-least-once. A provider that does not get a 2xx retries, so
// the SAME event arrives again — it must be a no-op, not a second application.
func TestIntegration_Apply_IsIdempotentOnEventID(t *testing.T) {
	db := testdb.NewTx(t)
	id := seedSend(t, db, "sent")
	a := emailevents.NewApplier(db, nil)
	ev := emailevents.Event{EventID: "evt_dupe", SendID: id, Type: "email.bounced", At: time.Now()}

	require.NoError(t, a.Apply(context.Background(), ev))
	require.NoError(t, a.Apply(context.Background(), ev), "a replayed event must not error")

	var n int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM email_send_events WHERE send_id = ?`, id).Scan(&n).Error)
	require.EqualValues(t, 1, n, "a replayed event must be recorded once")
	require.Equal(t, "bounced", statusOf(t, db, id))
}

// Events do not arrive in order. A `delivered` that lands after a `bounced`
// must not un-bounce the row — the bounce is the authoritative outcome, and
// regressing it would tell an operator mail arrived when it did not.
func TestIntegration_Apply_DoesNotRegressATerminalOutcome(t *testing.T) {
	db := testdb.NewTx(t)
	id := seedSend(t, db, "sent")
	a := emailevents.NewApplier(db, nil)

	require.NoError(t, a.Apply(context.Background(), emailevents.Event{
		EventID: "evt_b", SendID: id, Type: "email.bounced", At: time.Now()}))
	require.NoError(t, a.Apply(context.Background(), emailevents.Event{
		EventID: "evt_d", SendID: id, Type: "email.delivered", At: time.Now()}))

	require.Equal(t, "bounced", statusOf(t, db, id),
		"a late delivered event must not override a bounce")
}

// A delay is informational — the message is still in flight, and marking it
// anything terminal would be wrong.
func TestIntegration_Apply_DelayLeavesStatusAlone(t *testing.T) {
	db := testdb.NewTx(t)
	id := seedSend(t, db, "sent")
	a := emailevents.NewApplier(db, nil)

	require.NoError(t, a.Apply(context.Background(), emailevents.Event{
		EventID: "evt_z", SendID: id, Type: "email.delivery_delayed", At: time.Now()}))
	require.Equal(t, "sent", statusOf(t, db, id))
}

// An event for a send we never recorded is not an error worth retrying: the
// provider would redeliver forever. It is recorded and ignored.
func TestIntegration_Apply_UnknownSendIsNotAnError(t *testing.T) {
	db := testdb.NewTx(t)
	a := emailevents.NewApplier(db, nil)

	require.NoError(t, a.Apply(context.Background(), emailevents.Event{
		EventID: "evt_orphan", SendID: uuid.New(), Type: "email.delivered", At: time.Now()}))
}

// An unrecognised type is logged and ignored rather than rejected. A provider
// adding an event type must not start a retry storm against us.
func TestIntegration_Apply_UnknownTypeIsIgnored(t *testing.T) {
	db := testdb.NewTx(t)
	id := seedSend(t, db, "sent")
	a := emailevents.NewApplier(db, nil)

	require.NoError(t, a.Apply(context.Background(), emailevents.Event{
		EventID: "evt_new", SendID: id, Type: "email.something_new", At: time.Now()}))
	require.Equal(t, "sent", statusOf(t, db, id))
}
