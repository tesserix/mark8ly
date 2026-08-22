package giftcard

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
)

func sampleDeliveryInput() DeliveryInput {
	expires := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	sender := "Pat"
	recipient := "Sam"
	msg := "Enjoy!"
	return DeliveryInput{
		Recipient: "sam@example.com",
		TenantID:  "tenant-1",
		Card: &GiftCard{
			Code:           "GC-12345",
			InitialBalance: decimal.NewFromFloat(50.00),
			CurrencyCode:   "AUD",
			ExpiresAt:      &expires,
			SenderName:     &sender,
			RecipientName:  &recipient,
			Message:        &msg,
		},
		Theme:         GiftCardEmailTheme{StoreName: "Acme Store"},
		StorefrontURL: "https://acme.example",
	}
}

// TestRender_Embedded — embedded fallback path renders subject + body.
func TestRender_Embedded(t *testing.T) {
	subj, html, text, err := renderDelivery(context.Background(), nil, sampleDeliveryInput())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(subj, "Acme Store") {
		t.Errorf("subject = %q, want to contain Acme Store", subj)
	}
	if !strings.Contains(html, "GC-12345") {
		t.Errorf("html missing card code")
	}
	if !strings.Contains(text, "GC-12345") {
		t.Errorf("text missing card code")
	}
}

// TestRender_NilCard_Errors verifies guard against missing card.
func TestRender_NilCard_Errors(t *testing.T) {
	in := sampleDeliveryInput()
	in.Card = nil
	_, _, _, err := renderDelivery(context.Background(), nil, in)
	if err == nil {
		t.Fatal("expected error for nil card")
	}
}

// TestRegisterFallbacks_RegistersDeliveryKey — fallback must be
// registered under the documented key so the loader can find it.
func TestRegisterFallbacks_RegistersDeliveryKey(t *testing.T) {
	loader := emailtemplates.NewLoader(nil)
	RegisterFallbacks(loader)
	subj, html, text, err := renderDelivery(context.Background(), loader, sampleDeliveryInput())
	if err != nil {
		t.Fatalf("render via loader: %v", err)
	}
	if !strings.Contains(subj, "Acme Store") {
		t.Errorf("subject = %q", subj)
	}
	if html == "" || text == "" {
		t.Error("loader render produced empty body")
	}
}

// TestRender_LoaderPath_MatchesEmbedded — byte-identity proof.
func TestRender_LoaderPath_MatchesEmbedded(t *testing.T) {
	loader := emailtemplates.NewLoader(nil)
	RegisterFallbacks(loader)

	in := sampleDeliveryInput()
	embSub, embHTML, embText, err := renderDelivery(context.Background(), nil, in)
	if err != nil {
		t.Fatal(err)
	}
	loaderSub, loaderHTML, loaderText, err := renderDelivery(context.Background(), loader, in)
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

// TestDeliveryTemplateKey_Constant — pin the key so external code can
// rely on it. tesserix-home admin UI lists by key.
func TestDeliveryTemplateKey_Constant(t *testing.T) {
	if DeliveryTemplateKey != "giftcard_delivery" {
		t.Errorf("DeliveryTemplateKey = %q, want giftcard_delivery", DeliveryTemplateKey)
	}
}
