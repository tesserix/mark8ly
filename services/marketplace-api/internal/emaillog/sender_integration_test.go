//go:build integration

package emaillog_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/emaillog"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

type recordingSender struct {
	got  email.Message
	err  error
	sent int
}

func (r *recordingSender) Send(_ context.Context, m email.Message) error {
	r.sent++
	r.got = m
	return r.err
}

type row struct {
	ID        uuid.UUID
	TenantID  *uuid.UUID
	StoreID   *uuid.UUID
	Recipient string
	Kind      string
	Status    string
	Error     *string
	SentAt    *string
}

func rowsFor(t *testing.T, db *gorm.DB, recipient string) []row {
	t.Helper()
	var out []row
	require.NoError(t, db.Raw(
		`SELECT id, tenant_id, store_id, recipient, kind, status, error,
		        sent_at::text AS sent_at
		 FROM email_sends WHERE recipient = ? ORDER BY created_at`, recipient).
		Scan(&out).Error)
	return out
}

func msg(to string, args map[string]string) email.Message {
	return email.Message{To: to, Subject: "Your order #1234 from Acme Ltd",
		HTMLBody: "<p>secret body</p>", CustomArgs: args}
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(nopWriter{}, nil)) }

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

// Every send writes a row, and a successful one ends at `sent`.
func TestIntegration_EmailLog_SuccessfulSendIsRecorded(t *testing.T) {
	db := testdb.NewTx(t)
	inner := &recordingSender{}
	tenantID, storeID := uuid.New(), uuid.New()

	s := emaillog.NewSender(inner, db, quietLogger())
	to := uuid.NewString() + "@example.com"
	require.NoError(t, s.Send(context.Background(), msg(to, map[string]string{
		"product": "mark8ly", "kind": "giftcard",
		"tenant_id": tenantID.String(), "store_id": storeID.String(),
	})))

	rows := rowsFor(t, db, to)
	require.Len(t, rows, 1)
	require.Equal(t, "sent", rows[0].Status)
	require.Equal(t, "giftcard", rows[0].Kind)
	require.NotNil(t, rows[0].TenantID)
	require.Equal(t, tenantID, *rows[0].TenantID)
	require.NotNil(t, rows[0].StoreID)
	require.Equal(t, storeID, *rows[0].StoreID)
	require.NotNil(t, rows[0].SentAt, "a completed send records when it completed")
	require.Nil(t, rows[0].Error)
}

// A failed delivery is recorded too — that is the whole point of the log —
// with the provider's error, and the error is still returned to the caller.
func TestIntegration_EmailLog_FailedSendIsRecordedAndErrorPropagates(t *testing.T) {
	db := testdb.NewTx(t)
	inner := &recordingSender{err: errors.New("provider refused")}

	s := emaillog.NewSender(inner, db, quietLogger())
	to := uuid.NewString() + "@example.com"
	err := s.Send(context.Background(), msg(to, map[string]string{"kind": "ticket"}))
	require.Error(t, err, "the decorator must not swallow a delivery failure")

	rows := rowsFor(t, db, to)
	require.Len(t, rows, 1)
	require.Equal(t, "failed", rows[0].Status)
	require.NotNil(t, rows[0].Error)
	require.Contains(t, *rows[0].Error, "provider refused")
	require.Nil(t, rows[0].SentAt, "a failed send never completed")
}

// The most important test in this package: an observability feature must not
// be able to take down transactional mail. With the table unreachable, the
// email still goes out.
func TestIntegration_EmailLog_LogWriteFailureDoesNotBlockDelivery(t *testing.T) {
	db := testdb.NewTx(t)
	require.NoError(t, db.Exec(`ALTER TABLE email_sends RENAME TO email_sends_hidden`).Error)

	inner := &recordingSender{}
	s := emaillog.NewSender(inner, db, quietLogger())
	to := uuid.NewString() + "@example.com"

	require.NoError(t, s.Send(context.Background(), msg(to, map[string]string{"kind": "ticket"})),
		"a failed log write must not fail the send")
	require.Equal(t, 1, inner.sent, "the email must still have been dispatched")
}

// No customer content reaches the row. Asserted against STORED VALUES, not
// against the struct definition — a column added later would slip past a
// struct-shaped assertion.
func TestIntegration_EmailLog_NoSubjectOrBodyIsStored(t *testing.T) {
	db := testdb.NewTx(t)
	s := emaillog.NewSender(&recordingSender{}, db, quietLogger())
	to := uuid.NewString() + "@example.com"
	require.NoError(t, s.Send(context.Background(), msg(to, map[string]string{"kind": "orderdoc"})))

	var dump string
	require.NoError(t, db.Raw(
		`SELECT COALESCE(to_jsonb(e)::text, '') FROM email_sends e WHERE recipient = ?`, to).
		Scan(&dump).Error)
	require.NotContains(t, dump, "Acme Ltd", "the subject is interpolated customer content")
	require.NotContains(t, dump, "secret body", "the rendered body must never be stored")
}

// The send id reaches the provider as a custom arg — that is what makes a
// provider event correlatable in piece B — and the caller's map is never
// mutated, because it may be shared or reused.
func TestIntegration_EmailLog_InjectsSendIDWithoutMutatingCallerMap(t *testing.T) {
	db := testdb.NewTx(t)
	inner := &recordingSender{}
	callerArgs := map[string]string{"product": "mark8ly", "kind": "campaign"}

	s := emaillog.NewSender(inner, db, quietLogger())
	to := uuid.NewString() + "@example.com"
	require.NoError(t, s.Send(context.Background(), msg(to, callerArgs)))

	require.Len(t, callerArgs, 2, "the caller's CustomArgs map must not be mutated")
	_, leaked := callerArgs[emaillog.CustomArgSendID]
	require.False(t, leaked)

	sendID := inner.got.CustomArgs[emaillog.CustomArgSendID]
	require.NotEmpty(t, sendID, "the wrapped sender must receive the send id")

	rows := rowsFor(t, db, to)
	require.Len(t, rows, 1)
	require.Equal(t, rows[0].ID.String(), sendID,
		"the row's identity and the correlation key must be the SAME value")
}

// A mailer that supplies no attribution appears as unattributed and
// queryable, rather than writing an empty string nobody notices.
func TestIntegration_EmailLog_KindFallsBackToUnknown(t *testing.T) {
	db := testdb.NewTx(t)
	s := emaillog.NewSender(&recordingSender{}, db, quietLogger())
	to := uuid.NewString() + "@example.com"
	require.NoError(t, s.Send(context.Background(), msg(to, nil)))

	rows := rowsFor(t, db, to)
	require.Len(t, rows, 1)
	require.Equal(t, "unknown", rows[0].Kind)
	require.Nil(t, rows[0].TenantID, "platform-level mail legitimately has no tenant")
}
