//go:build integration

package instore_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/instore"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

type fakeReceiptMailer struct {
	sent []string // receipt URLs
}

func (m *fakeReceiptMailer) SendInStoreReceipt(_ context.Context, _ uuid.UUID, url string) error {
	m.sent = append(m.sent, url)
	return nil
}

func (f *fixture) cashSale(t *testing.T) uuid.UUID {
	t.Helper()
	vid := f.variant(t, "20.00", 5, "deny")
	in := f.sale(line(vid, 2))
	in.PaymentMethod = instore.PaymentCash
	in.CashTendered = dec("50")
	res, err := f.svc.RecordSale(context.Background(), in)
	require.NoError(t, err)
	return res.Order.ID
}

func (f *fixture) receipts(m instore.ReceiptMailer) *instore.ReceiptService {
	return instore.NewReceiptService(f.db, m, "https://{slug}.mark8ly.com")
}

func tokenOf(url string) string { return url[strings.LastIndex(url, "/")+1:] }

func TestReceipt_IssueAndLoad(t *testing.T) {
	f := newFixture(t)
	oid := f.cashSale(t)
	require.NoError(t, f.db.Exec(`UPDATE orders SET customer_email = 'jo@example.com' WHERE id = ?`, oid).Error)

	url, err := f.receipts(&fakeReceiptMailer{}).Issue(context.Background(), f.sid, oid)
	require.NoError(t, err)
	require.Regexp(t, `^https://[a-z0-9-]+\.mark8ly\.com/r/[A-Za-z0-9_-]{43}$`, url)

	r, err := f.receipts(nil).Load(context.Background(), f.sid, tokenOf(url))
	require.NoError(t, err)
	require.Len(t, r.Lines, 1)
	require.Equal(t, 2, r.Lines[0].Quantity)
	require.Equal(t, "40", r.GrandTotal.String())
	require.Equal(t, "GST included", r.TaxLabel)
	var tz string
	require.NoError(t, f.db.Raw(`SELECT timezone FROM stores WHERE id = ?`, f.sid).Row().Scan(&tz))
	require.Equal(t, tz, r.TimeZone, "the page shows the time where the shop is")
	require.True(t, r.Fresh)
	require.Equal(t, "cash", *r.PaymentMethod)
	require.Equal(t, "j***@example.com", *r.ReceiptEmailMasked)
}

func TestReceipt_StaleHidesContact(t *testing.T) {
	f := newFixture(t)
	oid := f.cashSale(t)
	url, err := f.receipts(nil).Issue(context.Background(), f.sid, oid)
	require.NoError(t, err)
	require.NoError(t, f.db.Exec(`UPDATE receipt_links SET created_at = now() - interval '8 days' WHERE order_id = ?`, oid).Error)

	r, err := f.receipts(nil).Load(context.Background(), f.sid, tokenOf(url))
	require.NoError(t, err)
	require.False(t, r.Fresh)
	require.Nil(t, r.PaymentMethod)
	require.Nil(t, r.ReceiptEmailMasked)
	require.Len(t, r.Lines, 1, "items stay visible for good")
}

func TestReceipt_UnknownToken404(t *testing.T) {
	f := newFixture(t)
	_, err := f.receipts(nil).Load(context.Background(), f.sid, "not-a-real-token")
	require.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestReceipt_OtherStore404(t *testing.T) {
	f := newFixture(t)
	url, err := f.receipts(nil).Issue(context.Background(), f.sid, f.cashSale(t))
	require.NoError(t, err)
	_, err = f.receipts(nil).Load(context.Background(), uuid.New(), tokenOf(url))
	require.ErrorIs(t, err, apperrors.ErrNotFound)
}

func TestReceiptResend_RateLimited(t *testing.T) {
	f := newFixture(t)
	oid := f.cashSale(t)
	require.NoError(t, f.db.Exec(`UPDATE orders SET customer_email = 'jo@example.com' WHERE id = ?`, oid).Error)
	url, err := f.receipts(nil).Issue(context.Background(), f.sid, oid)
	require.NoError(t, err)
	require.NoError(t, f.db.Exec(`UPDATE receipt_links SET created_at = now() - interval '8 days' WHERE order_id = ?`, oid).Error)

	m := &fakeReceiptMailer{}
	svc := f.receipts(m)
	require.NoError(t, svc.Resend(context.Background(), f.sid, tokenOf(url)))
	require.NoError(t, svc.Resend(context.Background(), f.sid, tokenOf(url)))
	require.Len(t, m.sent, 1, "one email per 10 minutes")
	require.NotEqual(t, url, m.sent[0])

	var n int
	require.NoError(t, f.db.Raw(`SELECT count(*) FROM receipt_links WHERE order_id = ?`, oid).Row().Scan(&n))
	require.Equal(t, 2, n)
}

func TestReceiptResend_NoEmailSendsNothing(t *testing.T) {
	f := newFixture(t)
	oid := f.cashSale(t)
	url, err := f.receipts(nil).Issue(context.Background(), f.sid, oid)
	require.NoError(t, err)
	require.NoError(t, f.db.Exec(`UPDATE receipt_links SET created_at = now() - interval '8 days' WHERE order_id = ?`, oid).Error)
	m := &fakeReceiptMailer{}
	require.NoError(t, f.receipts(m).Resend(context.Background(), f.sid, tokenOf(url)))
	require.Empty(t, m.sent)
}

func TestReceiptSend_SavesEmailAndMails(t *testing.T) {
	f := newFixture(t)
	oid := f.cashSale(t)
	m := &fakeReceiptMailer{}
	url, err := f.receipts(m).Send(context.Background(), f.sid, oid, "  Jo@Example.com ")
	require.NoError(t, err)
	require.Equal(t, []string{url}, m.sent)
	var got string
	require.NoError(t, f.db.Raw(`SELECT customer_email FROM orders WHERE id = ?`, oid).Row().Scan(&got))
	require.Equal(t, "jo@example.com", got)
}

func TestReceiptSend_RefusesBadEmail(t *testing.T) {
	f := newFixture(t)
	_, err := f.receipts(&fakeReceiptMailer{}).Send(context.Background(), f.sid, f.cashSale(t), "not-an-email")
	require.Error(t, err)
}
