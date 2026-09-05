// Package billing — reverse_charge_invoice.go: §19.2 Stripe `custom_fields`
// annotation for B2B invoices in jurisdictions where the recipient self-accounts
// for VAT/GST. Annotation is a no-op for AU (Tesserix is not registered for GST
// and charges none, so there is no GST for a recipient to self-account for —
// this supersedes §19.4, which assumed registration) and US (no federal
// sales-tax reverse-charge concept).
package billing

import (
	"context"
	"fmt"
)

// ReverseChargeCountries are the jurisdictions whose validated B2B invoices
// must carry the reverse-charge clause. AU and US are intentionally excluded;
// CA uses attestation rather than VAT and is also excluded. AU's exclusion was
// decided under the unregistered position above and must be RE-EXAMINED if
// Tesserix registers for GST, not inherited as settled.
var ReverseChargeCountries = map[string]bool{
	"GB": true,
	"IE": true, "DE": true, "FR": true, "IT": true, "ES": true, "NL": true,
	"IN": true, "SG": true,
	"MY": true, "TH": true, "PH": true, "ID": true, "VN": true,
	"NZ": true,
}

// CustomField is the (name, value) pair pushed onto a Stripe invoice. Mirrors
// the SDK's InvoiceUpdateCustomFieldParams shape but stays SDK-free so the
// annotator is testable without importing stripe-go.
type CustomField struct {
	Name  string
	Value string
}

// InvoiceCustomFieldUpdater is the narrow Stripe contract this annotator
// requires. Implementations live in internal/billing/stripe.
type InvoiceCustomFieldUpdater interface {
	UpdateInvoiceCustomFields(ctx context.Context, invoiceID string, fields []CustomField) error
}

// AnnotateInput is what the webhook passes us. ReverseChargeTaxID is rendered
// into the clause text so the recipient's accountant can match the invoice to
// the registered identity.
type AnnotateInput struct {
	InvoiceID          string
	Country            string
	TaxIDValidated     bool
	ReverseChargeTaxID string
}

// ReverseChargeAnnotator is a thin policy layer over the Stripe SDK.
type ReverseChargeAnnotator struct {
	stripe InvoiceCustomFieldUpdater
}

// NewReverseChargeAnnotator constructs an annotator. Caller owns the lifetime
// of the underlying Stripe client.
func NewReverseChargeAnnotator(c InvoiceCustomFieldUpdater) *ReverseChargeAnnotator {
	return &ReverseChargeAnnotator{stripe: c}
}

// AnnotateIfNeeded is a no-op unless (a) the country supports reverse charge
// AND (b) the tax ID has been registry-validated. Safe to call from every
// invoice.finalized webhook regardless of jurisdiction or validation state.
func (a *ReverseChargeAnnotator) AnnotateIfNeeded(ctx context.Context, in AnnotateInput) error {
	if a == nil || a.stripe == nil {
		return nil
	}
	if !ReverseChargeCountries[in.Country] || !in.TaxIDValidated {
		return nil
	}
	if in.InvoiceID == "" {
		return fmt.Errorf("billing: AnnotateIfNeeded: empty invoice id")
	}
	clause := fmt.Sprintf("Reverse charge — VAT/GST to be accounted for by the recipient. Tax ID: %s", in.ReverseChargeTaxID)
	return a.stripe.UpdateInvoiceCustomFields(ctx, in.InvoiceID, []CustomField{
		{Name: "Tax Treatment", Value: clause},
	})
}
