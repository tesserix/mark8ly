package payment

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Service orchestrates payment intent creation, webhook processing, and
// refunds. It delegates provider-specific work to the Gateway interface
// and persists results via the Repository.
type Service struct {
	repo Repository
}

// NewService returns a payment Service backed by the given repository.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// CreatePaymentIntent creates a payment intent with the provider and
// persists the resulting transaction record.
func (s *Service) CreatePaymentIntent(
	ctx context.Context,
	orderID string,
	amount decimal.Decimal,
	currency string,
	email string,
	gateway Gateway,
) (*Intent, error) {
	in := CreateIntentInput{
		OrderID:       orderID,
		Amount:        amount,
		CurrencyCode:  currency,
		CustomerEmail: email,
		Description:   fmt.Sprintf("Payment for order %s", orderID),
		Metadata:      map[string]string{"order_id": orderID},
	}

	intent, err := gateway.CreateIntent(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("payment service: create intent: %w", err)
	}

	tx := PaymentTransaction{
		OrderID:          orderID,
		Provider:         gateway.ProviderName(),
		ProviderIntentID: intent.ProviderIntentID,
		Amount:           amount,
		CurrencyCode:     currency,
		Status:           intent.Status,
	}
	if err := s.repo.CreateTransaction(ctx, &tx); err != nil {
		return nil, fmt.Errorf("payment service: persist transaction: %w", err)
	}

	return intent, nil
}

// ProcessWebhook verifies and processes a provider webhook callback.
// It persists the event for idempotency and updates the related
// transaction status.
func (s *Service) ProcessWebhook(
	ctx context.Context,
	provider string,
	payload []byte,
	signature string,
	gateway Gateway,
) (*WebhookEvent, error) {
	evt, err := gateway.VerifyWebhook(ctx, payload, signature)
	if err != nil {
		return nil, fmt.Errorf("payment service: process webhook: %w", err)
	}

	// Check idempotency — skip if we have already processed this event.
	existing, _ := s.repo.GetWebhookEventByProviderID(ctx, evt.ProviderEventID)
	if existing != nil {
		return evt, nil
	}

	record := WebhookEventRecord{
		Provider:        provider,
		ProviderEventID: evt.ProviderEventID,
		EventType:       evt.EventType,
		OrderID:         evt.OrderID,
		RawPayload:      evt.RawPayload,
	}
	if err := s.repo.CreateWebhookEvent(ctx, &record); err != nil {
		return nil, fmt.Errorf("payment service: persist webhook event: %w", err)
	}

	// Update the corresponding transaction status when we can correlate it.
	if evt.OrderID != "" {
		statusUpdate := webhookEventToStatus(evt.EventType)
		if statusUpdate != "" {
			if err := s.repo.UpdateTransactionStatus(ctx, evt.OrderID, statusUpdate); err != nil {
				return nil, fmt.Errorf("payment service: update transaction: %w", err)
			}
		}
	}

	return evt, nil
}

// ReserveRefundInput describes a refund reservation — the first step of the
// refund saga (ledger row inserted before any provider call is made). It
// carries CurrencyCode for later gateway use; refund_transactions has no
// currency column, so it is not persisted on the row.
type ReserveRefundInput struct {
	TenantID, StoreID, OrderID  string
	Provider, ProviderPaymentID string
	Amount                      decimal.Decimal
	CurrencyCode, Reason        string
	IdempotencyKey              string
}

// ReserveRefund inserts a pending refund ledger row inside tx, keyed by
// IdempotencyKey. This is the saga re-entry guard: replaying the same
// reservation (e.g. after a crash mid-saga) returns the original row with
// created=false instead of inserting a duplicate.
func (s *Service) ReserveRefund(ctx context.Context, tx *gorm.DB, in ReserveRefundInput) (*RefundTransaction, bool, error) {
	row, created, err := s.repo.InsertRefundPending(tx, &RefundTransaction{
		TenantID:          in.TenantID,
		StoreID:           in.StoreID,
		OrderID:           in.OrderID,
		Provider:          in.Provider,
		ProviderPaymentID: in.ProviderPaymentID,
		Amount:            in.Amount,
		Reason:            in.Reason,
		Status:            "pending",
		IdempotencyKey:    in.IdempotencyKey,
	})
	if err != nil {
		return nil, false, fmt.Errorf("payment service: reserve refund: %w", err)
	}
	return row, created, nil
}

// ExecuteGatewayRefund issues the refund against the provider. It is a pure
// gateway call — no DB access — so the saga orchestrator can call it outside
// any transaction and retry independently of ledger state.
func (s *Service) ExecuteGatewayRefund(ctx context.Context, gw Gateway, in RefundInput) (*Refund, error) {
	r, err := gw.RefundPayment(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("payment service: gateway refund: %w", err)
	}
	return r, nil
}

// FinalizeRefund records the gateway outcome on the ledger row, completing
// the saga.
func (s *Service) FinalizeRefund(ctx context.Context, tx *gorm.DB, ledgerID, providerRefundID, status string) error {
	if err := s.repo.UpdateRefundOutcome(tx, ledgerID, providerRefundID, status); err != nil {
		return fmt.Errorf("payment service: finalize refund: %w", err)
	}
	return nil
}

// MarkRefundFailed flags a reserved-but-unfulfillable refund ledger row as
// permanently 'failed', so the retry sweeper (which only re-drives 'pending'
// rows) stops re-driving it and it surfaces for manual reconciliation. The
// UPDATE is status-guarded to 'pending' so a row that concurrently raced to
// 'succeeded' is never clobbered. db may be a plain *gorm.DB (no surrounding
// transaction needed — this is a single guarded UPDATE).
func (s *Service) MarkRefundFailed(ctx context.Context, db *gorm.DB, ledgerID string) error {
	res := db.WithContext(ctx).Exec(
		`UPDATE refund_transactions SET status = 'failed', updated_at = now()
		  WHERE id = ? AND status = 'pending'`,
		ledgerID,
	)
	if res.Error != nil {
		return fmt.Errorf("payment service: mark refund failed: %w", res.Error)
	}
	return nil
}

func webhookEventToStatus(eventType string) string {
	switch eventType {
	case "payment.succeeded":
		return "captured"
	case "payment.failed":
		return "failed"
	case "refund.succeeded":
		return "refunded"
	default:
		return ""
	}
}
