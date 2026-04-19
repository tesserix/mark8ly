package dispatch

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing"
	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// stripeInvoiceUpdaterAdapter satisfies billing.InvoiceCustomFieldUpdater on
// top of *billingstripe.Client. Mapped here (not in the stripe package) so
// neither billing nor stripe needs to import the other.
type stripeInvoiceUpdaterAdapter struct {
	client *billingstripe.Client
}

func (a *stripeInvoiceUpdaterAdapter) UpdateInvoiceCustomFields(ctx context.Context, invoiceID string, fields []billing.CustomField) error {
	if a == nil || a.client == nil {
		return nil
	}
	mapped := make([]billingstripe.InvoiceCustomField, len(fields))
	for i, f := range fields {
		mapped[i] = billingstripe.InvoiceCustomField{Name: f.Name, Value: f.Value}
	}
	return billingstripe.UpdateInvoiceCustomFields(ctx, a.client, invoiceID, mapped)
}

// WithReverseChargeAnnotator wires a reverse-charge annotator onto the
// dispatcher and registers an `invoice.finalized` handler that calls it.
// Pass a *billingstripe.Client for the production wiring; tests can use
// (*Dispatcher).withReverseChargeAnnotator with a mock annotator directly.
func (d *Dispatcher) WithReverseChargeAnnotator(client *billingstripe.Client) *Dispatcher {
	annot := billing.NewReverseChargeAnnotator(&stripeInvoiceUpdaterAdapter{client: client})
	d.handlers["invoice.finalized"] = makeInvoiceFinalizedHandler(annot)
	return d
}

// makeInvoiceFinalizedHandler returns a handler that looks up the subscription
// by Stripe customer ID and, if tax_id_validated is true and the country
// supports reverse charge, annotates the invoice. Failures are non-fatal:
// they log but never block the webhook ack (annotation is cosmetic, not
// settlement-critical).
func makeInvoiceFinalizedHandler(annot *billing.ReverseChargeAnnotator) Handler {
	return func(ctx context.Context, tx *gorm.DB, raw []byte) error {
		invoiceID, customer, err := extractFinalizedInvoiceFields(raw)
		if err != nil {
			return nil // malformed payload — no-op; replay won't be useful.
		}
		if invoiceID == "" || customer == "" {
			return nil
		}

		var sub subscription.StoreSubscription
		if err := tx.WithContext(ctx).
			Where("stripe_customer_id = ?", customer).
			First(&sub).Error; err != nil {
			return nil // no matching subscription — likely test/fixture event.
		}
		if !sub.TaxIDValidated || sub.TaxIDCountry == nil || sub.ReverseChargeTaxID == nil {
			return nil
		}

		_ = annot.AnnotateIfNeeded(ctx, billing.AnnotateInput{
			InvoiceID:          invoiceID,
			Country:            *sub.TaxIDCountry,
			TaxIDValidated:     true,
			ReverseChargeTaxID: *sub.ReverseChargeTaxID,
		})
		return nil
	}
}

func extractFinalizedInvoiceFields(raw []byte) (invoiceID, customer string, err error) {
	customer, err = extractCustomerID(raw)
	if err != nil {
		return "", "", err
	}
	id, err := extractInvoiceID(raw)
	if err != nil {
		return "", "", err
	}
	return id, customer, nil
}

func extractInvoiceID(raw []byte) (string, error) {
	var e struct {
		Data struct {
			Object struct {
				ID string `json:"id"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return "", err
	}
	if e.Data.Object.ID == "" {
		return "", fmt.Errorf("dispatch: missing invoice id")
	}
	return e.Data.Object.ID, nil
}
