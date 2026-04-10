package payment

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"
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

// RefundPayment issues a refund through the provider and persists a
// refund transaction record.
func (s *Service) RefundPayment(
	ctx context.Context,
	providerPaymentID string,
	amount decimal.Decimal,
	reason string,
	gateway Gateway,
) (*Refund, error) {
	in := RefundInput{
		ProviderPaymentID: providerPaymentID,
		Amount:            amount,
		Reason:            reason,
	}

	refund, err := gateway.RefundPayment(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("payment service: refund: %w", err)
	}

	record := RefundTransaction{
		ProviderPaymentID: providerPaymentID,
		ProviderRefundID:  refund.ProviderRefundID,
		Provider:          gateway.ProviderName(),
		Amount:            amount,
		Reason:            reason,
		Status:            refund.Status,
	}
	if err := s.repo.CreateRefundTransaction(ctx, &record); err != nil {
		return nil, fmt.Errorf("payment service: persist refund: %w", err)
	}

	return refund, nil
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
