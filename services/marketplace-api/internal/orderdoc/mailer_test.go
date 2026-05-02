package orderdoc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
)

func sampleInput(orderNum string) DocumentInput {
	placed, _ := time.Parse(time.RFC3339, "2026-04-15T10:30:00Z")
	return DocumentInput{
		Recipient:      "buyer@example.com",
		TenantID:       "tenant-1",
		DocumentNumber: "INV-PLA-260415-00001",
		OrderID:        "ord-uuid",
		StoreSlug:      "acme",
		OrderNumber:    orderNum,
		OrderURL:       "https://acme.example/account/orders/ord-uuid",
		DocumentURL:    "https://acme.example/api/orders/ord-uuid/invoice",
		PlacedAt:       placed,
		GrandTotal:     decimal.NewFromFloat(99.95),
		CurrencyCode:   "AUD",
		ItemCount:      2,
		Theme:          Theme{StoreName: "Acme Store"},
	}
}

// TestRender_Invoice_Embedded — embedded fallback path returns the
// expected subject and body content for an invoice.
func TestRender_Invoice_Embedded(t *testing.T) {
	in := sampleInput("ORD-1234")
	subject, html, text, err := render(context.Background(), nil, KindInvoice, in, false)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(subject, "ORD-1234") {
		t.Errorf("subject missing OrderNumber: %q", subject)
	}
	if !strings.Contains(subject, "confirmed") {
		t.Errorf("subject not the invoice subject: %q", subject)
	}
	if !strings.Contains(html, "ORD-1234") {
		t.Errorf("html missing OrderNumber")
	}
	if !strings.Contains(text, "ORD-1234") {
		t.Errorf("text missing OrderNumber")
	}
}

// TestRender_Refund_FullVsPartial — subject template branches on
// IsFullRefund. Full refund and partial refund should produce
// different subject lines.
func TestRender_Refund_FullVsPartial(t *testing.T) {
	full := sampleInput("ORD-9000")
	full.IsFullRefund = true
	full.RefundAmount = decimal.NewFromFloat(99.95)
	full.TotalRefunded = decimal.NewFromFloat(99.95)

	partial := sampleInput("ORD-9001")
	partial.IsFullRefund = false
	partial.RefundAmount = decimal.NewFromFloat(20.00)
	partial.TotalRefunded = decimal.NewFromFloat(20.00)

	fullSub, _, _, err := render(context.Background(), nil, KindRefund, full, false)
	if err != nil {
		t.Fatalf("render full: %v", err)
	}
	partialSub, _, _, err := render(context.Background(), nil, KindRefund, partial, false)
	if err != nil {
		t.Fatalf("render partial: %v", err)
	}
	if !strings.Contains(fullSub, "fully refunded") {
		t.Errorf("full refund subject = %q, want to contain 'fully refunded'", fullSub)
	}
	if !strings.Contains(partialSub, "Partial") {
		t.Errorf("partial refund subject = %q, want to contain 'Partial'", partialSub)
	}
	if fullSub == partialSub {
		t.Error("full and partial refund subjects should differ")
	}
}

// TestRender_Cancellation_CustomerVsAdmin — lede branches on
// CancelledByCustomer.
func TestRender_Cancellation_CustomerVsAdmin(t *testing.T) {
	customer := sampleInput("ORD-CUST")
	customer.CancelledByCustomer = true
	admin := sampleInput("ORD-ADM")
	admin.CancelledByCustomer = false

	_, custHTML, _, err := render(context.Background(), nil, KindCancellation, customer, false)
	if err != nil {
		t.Fatalf("render cust: %v", err)
	}
	_, admHTML, _, err := render(context.Background(), nil, KindCancellation, admin, false)
	if err != nil {
		t.Fatalf("render adm: %v", err)
	}
	if !strings.Contains(custHTML, "at your request") {
		t.Errorf("customer cancellation should mention 'at your request'")
	}
	if !strings.Contains(admHTML, "Acme Store") {
		t.Errorf("admin cancellation should mention store name")
	}
}

// TestTemplateKey_AllKinds — keys must be unique per kind so the
// loader doesn't cross-wire templates.
func TestTemplateKey_AllKinds(t *testing.T) {
	keys := map[string]bool{}
	for _, k := range []Kind{KindInvoice, KindReceipt, KindCancellation, KindRefund} {
		key := templateKey(k)
		if keys[key] {
			t.Errorf("duplicate key %q for kind %q", key, k)
		}
		keys[key] = true
	}
	if len(keys) != 4 {
		t.Errorf("expected 4 distinct keys, got %d", len(keys))
	}
}

// TestRegisterFallbacks_RegistersAllKinds — every kind must have
// an embedded fallback after RegisterFallbacks runs.
func TestRegisterFallbacks_RegistersAllKinds(t *testing.T) {
	loader := emailtemplates.NewLoader(nil)
	RegisterFallbacks(loader)
	in := sampleInput("ORD-X")
	for _, k := range []Kind{KindInvoice, KindReceipt, KindCancellation, KindRefund} {
		if _, _, _, err := render(context.Background(), loader, k, in, false); err != nil {
			t.Errorf("render %q: %v", k, err)
		}
	}
}

// TestRender_LoaderPath_MatchesEmbedded — byte-identity check via
// the loader's nil-DB code path. The loader.Render(nil DB) must
// produce the same output as direct embedded rendering.
func TestRender_LoaderPath_MatchesEmbedded(t *testing.T) {
	loader := emailtemplates.NewLoader(nil)
	RegisterFallbacks(loader)

	in := sampleInput("ORD-EQ")
	in.IsFullRefund = true
	in.RefundAmount = decimal.NewFromFloat(50.00)
	in.TotalRefunded = decimal.NewFromFloat(50.00)

	embSub, embHTML, embText, err := render(context.Background(), nil, KindRefund, in, false)
	if err != nil {
		t.Fatal(err)
	}
	loaderSub, loaderHTML, loaderText, err := render(context.Background(), loader, KindRefund, in, false)
	if err != nil {
		t.Fatal(err)
	}
	if embSub != loaderSub {
		t.Errorf("subject drift:\n emb: %q\n ld:  %q", embSub, loaderSub)
	}
	if embHTML != loaderHTML {
		t.Error("html drift between embedded and loader paths")
	}
	if embText != loaderText {
		t.Errorf("text drift:\n emb: %q\n ld:  %q", embText, loaderText)
	}
}
