//go:build integration

package storefront_test

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/branding"
	"github.com/mark8ly/marketplace-api/internal/handlers/storefront"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/orderdoc"
	"github.com/mark8ly/marketplace-api/internal/orderrefund"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/payment"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// recordingMailer captures the orderdoc envelopes a handler dispatches.
// Both sends are fire-and-forget goroutines, so each call is announced on a
// buffered channel the test waits on rather than sleeping.
type recordingMailer struct {
	refunds       chan orderdoc.DocumentInput
	cancellations chan orderdoc.DocumentInput
}

func newRecordingMailer() *recordingMailer {
	return &recordingMailer{
		refunds:       make(chan orderdoc.DocumentInput, 4),
		cancellations: make(chan orderdoc.DocumentInput, 4),
	}
}

func (m *recordingMailer) SendRefund(_ context.Context, in orderdoc.DocumentInput) error {
	m.refunds <- in
	return nil
}

func (m *recordingMailer) SendCancellation(_ context.Context, in orderdoc.DocumentInput) error {
	m.cancellations <- in
	return nil
}

// The rest of the Mailer contract is unused by the cancel path.
func (m *recordingMailer) SendInvoice(context.Context, orderdoc.DocumentInput) error { return nil }
func (m *recordingMailer) SendReceipt(context.Context, orderdoc.DocumentInput) error { return nil }
func (m *recordingMailer) SendShipmentDispatched(context.Context, orderdoc.DocumentInput) error {
	return nil
}

// A customer who self-cancels a PAID order gets their money back, but until
// #169 they were told only "your order was cancelled" — no refund
// confirmation. The admin-initiated refund has always sent one
// (handlers/admin/orders.go dispatchRefundEmail), so the same customer got a
// different experience depending on who pressed the button.
func TestIntegration_SelfCancelPaid_SendsRefundEmailWithRunningTotal(t *testing.T) {
	env, mailer := setupCancelRefundEmailEnv(t)
	cust := seedCancelAutoRefundCustomer(t, env.db, env.store)
	o := seedCancelAutoRefundOrder(t, env.db, env.store, cust,
		cancelAutoRefundOrderOpts{GrandTotal: "120.00"})
	seedCancelAutoRefundCapturedTxn(t, env.db, env.store, o.ID, "120.00")
	seedCancelAutoRefundGatewayConfig(t, env.db, env.store)

	r := buildCancelRouter(env.handler, env.store, cust)
	if w := cancelAutoRefundRequest(r, env.store.Slug, o.ID.String()); w.Code != http.StatusOK {
		t.Fatalf("cancel: status %d, body=%s", w.Code, w.Body.String())
	}

	select {
	case in := <-mailer.refunds:
		if !in.RefundAmount.Equal(decimal.RequireFromString("120.00")) {
			t.Errorf("RefundAmount = %s, want 120.00", in.RefundAmount)
		}
		// The running total must reflect THIS refund. The handler loads the
		// order before refunding, so a naive pass-through would report 0.00
		// and tell the customer nothing had been refunded yet.
		if !in.TotalRefunded.Equal(decimal.RequireFromString("120.00")) {
			t.Errorf("TotalRefunded = %s, want 120.00 — the post-refund total, not the pre-refund row",
				in.TotalRefunded)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("no refund email dispatched after a paid self-cancel")
	}

	// This ADDS an email; it does not replace the cancellation one.
	select {
	case <-mailer.cancellations:
	case <-time.After(15 * time.Second):
		t.Fatal("the cancellation email must still be sent")
	}
}

// An unpaid order is never refunded, so it must never claim to be. Telling a
// customer money came back when none moved is worse than sending nothing.
func TestIntegration_SelfCancelUnpaid_SendsNoRefundEmail(t *testing.T) {
	env, mailer := setupCancelRefundEmailEnv(t)
	cust := seedCancelAutoRefundCustomer(t, env.db, env.store)
	o := seedCancelAutoRefundOrder(t, env.db, env.store, cust,
		cancelAutoRefundOrderOpts{GrandTotal: "80.00", PaymentStatus: string(order.PaymentStatusPending)})

	r := buildCancelRouter(env.handler, env.store, cust)
	if w := cancelAutoRefundRequest(r, env.store.Slug, o.ID.String()); w.Code != http.StatusOK {
		t.Fatalf("cancel: status %d, body=%s", w.Code, w.Body.String())
	}

	// Waiting for the cancellation email first proves the handler ran to
	// completion, so the absent refund email below is a real absence rather
	// than a race that has not finished yet.
	select {
	case <-mailer.cancellations:
	case <-time.After(15 * time.Second):
		t.Fatal("the cancellation email must still be sent for an unpaid order")
	}
	select {
	case in := <-mailer.refunds:
		t.Fatalf("an unpaid self-cancel must not send a refund email; got amount %s", in.RefundAmount)
	case <-time.After(2 * time.Second):
	}
}

// setupCancelRefundEmailEnv mirrors setupCancelAutoRefundEnv but wires a real
// orderdoc.Service over a recording Mailer, so the email dispatch the other
// file passes nil for is actually observable.
func setupCancelRefundEmailEnv(t *testing.T) (*cancelAutoRefundEnv, *recordingMailer) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.NewDB(t, cancelAutoRefundTables...)
	store := seedCancelAutoRefundStore(t, db)

	orderRepo := order.NewRepository()
	orderSvc := order.NewService(db, orderRepo, outbox.NewRepository(db))
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	gw := &carFakeGateway{}
	res := &carFakeRefundResolver{Resolver: orderrefund.NewResolver(db), gw: gw}
	paySvc := payment.NewService(payment.NewRepository(db))
	coordinator := orderrefund.NewCoordinator(db, res, paySvc, orderSvc, orderRepo, true)

	mailer := newRecordingMailer()
	// orderdoc.Service dereferences brandingSvc in buildInput, so a nil here
	// panics inside the dispatch goroutine rather than failing the test.
	brandingSvc := branding.NewService(branding.ServiceConfig{
		DB: db, Repo: branding.NewRepository(), Logger: logger,
	})
	docSvc := orderdoc.NewService(db, mailer, orderRepo, brandingSvc, "https://example.test")

	handler := storefront.NewOrderDetailHandler(db, orderRepo, orderSvc, docSvc, logger).
		WithRefunds(coordinator)

	return &cancelAutoRefundEnv{db: db, handler: handler, store: store}, mailer
}
