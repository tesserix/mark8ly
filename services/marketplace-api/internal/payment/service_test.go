package payment

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// fakeGateway — implements the full Gateway interface for unit tests. Only
// RefundPayment is exercised; the rest return zero values.
// ---------------------------------------------------------------------------

type fakeGateway struct {
	refund    *Refund
	refundErr error
	lastIn    RefundInput
}

func (f *fakeGateway) CreateIntent(ctx context.Context, in CreateIntentInput) (*Intent, error) {
	return nil, nil
}

func (f *fakeGateway) CapturePayment(ctx context.Context, captureID string) (*Capture, error) {
	return nil, nil
}

func (f *fakeGateway) RefundPayment(ctx context.Context, in RefundInput) (*Refund, error) {
	f.lastIn = in
	if f.refundErr != nil {
		return nil, f.refundErr
	}
	return f.refund, nil
}

func (f *fakeGateway) VerifyWebhook(ctx context.Context, payload []byte, signature string) (*WebhookEvent, error) {
	return nil, nil
}

func (f *fakeGateway) ProviderName() string { return "fake" }

func (f *fakeGateway) SupportedCountries() []string { return nil }

// ---------------------------------------------------------------------------
// fakeRepo — implements the full Repository interface for unit tests.
// Refund-ledger methods are backed by an in-memory map keyed by idempotency
// key so ReserveRefund's conflict-detection behavior can be exercised
// without a real database.
// ---------------------------------------------------------------------------

type fakeRepo struct {
	byIdempotencyKey map[string]*RefundTransaction
	nextID           int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byIdempotencyKey: make(map[string]*RefundTransaction)}
}

func (f *fakeRepo) CreateTransaction(ctx context.Context, tx *PaymentTransaction) error {
	return nil
}

func (f *fakeRepo) GetTransactionByOrderID(ctx context.Context, orderID string) (*PaymentTransaction, error) {
	return nil, nil
}

func (f *fakeRepo) UpdateTransactionStatus(ctx context.Context, orderID, status string) error {
	return nil
}

func (f *fakeRepo) CreateRefundTransaction(ctx context.Context, refund *RefundTransaction) error {
	return nil
}

func (f *fakeRepo) ListRefundsByPaymentID(ctx context.Context, providerPaymentID string) ([]RefundTransaction, error) {
	return nil, nil
}

func (f *fakeRepo) InsertRefundPending(tx *gorm.DB, row *RefundTransaction) (*RefundTransaction, bool, error) {
	// Not used directly by unit tests below — repository-level fake logic
	// isn't exercised because ReserveRefund's idempotency guarantee is
	// verified against a real database in service_integration_test.go
	// (the ON CONFLICT DO NOTHING semantics require real Postgres).
	if row.Status == "" {
		row.Status = "pending"
	}
	if existing, ok := f.byIdempotencyKey[row.IdempotencyKey]; ok {
		return existing, false, nil
	}
	f.nextID++
	row.ID = row.IdempotencyKey // deterministic fake id for equality checks
	f.byIdempotencyKey[row.IdempotencyKey] = row
	return row, true, nil
}

func (f *fakeRepo) UpdateRefundOutcome(tx *gorm.DB, ledgerID, providerRefundID, status string) error {
	return nil
}

func (f *fakeRepo) GetRefundByIdempotencyKey(ctx context.Context, key string) (*RefundTransaction, error) {
	if row, ok := f.byIdempotencyKey[key]; ok {
		return row, nil
	}
	return nil, errors.New("not found")
}

// ---------------------------------------------------------------------------
// ExecuteGatewayRefund — pure gateway call, no DB.
// ---------------------------------------------------------------------------

func TestExecuteGatewayRefund_PassesIdempotencyKey(t *testing.T) {
	fg := &fakeGateway{refund: &Refund{ProviderRefundID: "re_9", Status: "succeeded"}}
	svc := NewService(newFakeRepo())

	refund, err := svc.ExecuteGatewayRefund(context.Background(), fg, RefundInput{
		ProviderPaymentID: "pi_1",
		Amount:            decimal.NewFromInt(10),
		IdempotencyKey:    "k1",
	})
	if err != nil {
		t.Fatalf("ExecuteGatewayRefund: unexpected error: %v", err)
	}
	if fg.lastIn.IdempotencyKey != "k1" {
		t.Fatalf("gateway got IdempotencyKey %q, want %q", fg.lastIn.IdempotencyKey, "k1")
	}
	if refund == nil || refund.ProviderRefundID != "re_9" {
		t.Fatalf("ExecuteGatewayRefund returned %+v, want ProviderRefundID re_9", refund)
	}
}

func TestExecuteGatewayRefund_WrapsGatewayError(t *testing.T) {
	fg := &fakeGateway{refundErr: errors.New("gateway down")}
	svc := NewService(newFakeRepo())

	refund, err := svc.ExecuteGatewayRefund(context.Background(), fg, RefundInput{
		ProviderPaymentID: "pi_1",
		Amount:            decimal.NewFromInt(10),
		IdempotencyKey:    "k1",
	})
	if err == nil {
		t.Fatal("ExecuteGatewayRefund: expected error, got nil")
	}
	if refund != nil {
		t.Fatalf("ExecuteGatewayRefund: expected nil refund on error, got %+v", refund)
	}
	if !errors.Is(err, fg.refundErr) {
		t.Fatalf("ExecuteGatewayRefund: error %v does not wrap gateway error %v", err, fg.refundErr)
	}
}
