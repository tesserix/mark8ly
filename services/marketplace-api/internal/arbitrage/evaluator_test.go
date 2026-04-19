package arbitrage

import (
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

func TestEvaluate_PPPTierWithDevelopedCardIsFlagged(t *testing.T) {
	in := Input{
		PriceTier:      subscription.PriceTierPPP,
		CardCountry:    "US",
		BillingCountry: "IN",
		IPCountry:      "IN",
	}
	got := Evaluate(in)
	if !got.Flagged {
		t.Fatal("expected Flagged=true, got false")
	}
	if !strings.Contains(got.MismatchReason, "card_country=US") {
		t.Errorf("MismatchReason %q does not contain 'card_country=US'", got.MismatchReason)
	}
	if !strings.Contains(got.MismatchReason, "developed") {
		t.Errorf("MismatchReason %q does not contain 'developed'", got.MismatchReason)
	}
}

func TestEvaluate_PPPTierWithDevelopedIPIsFlagged(t *testing.T) {
	in := Input{
		PriceTier:      subscription.PriceTierPPP,
		CardCountry:    "IN",
		BillingCountry: "IN",
		IPCountry:      "GB",
	}
	got := Evaluate(in)
	if !got.Flagged {
		t.Fatal("expected Flagged=true, got false")
	}
	if !strings.Contains(got.MismatchReason, "ip_country=GB") {
		t.Errorf("MismatchReason %q does not contain 'ip_country=GB'", got.MismatchReason)
	}
}

func TestEvaluate_PPPTierFullyEmergingIsClean(t *testing.T) {
	in := Input{
		PriceTier:      subscription.PriceTierPPP,
		CardCountry:    "IN",
		BillingCountry: "IN",
		IPCountry:      "IN",
	}
	got := Evaluate(in)
	if got.Flagged {
		t.Errorf("expected Flagged=false, got true; reason=%q", got.MismatchReason)
	}
	if got.MismatchReason != "" {
		t.Errorf("expected empty MismatchReason, got %q", got.MismatchReason)
	}
}

func TestEvaluate_DevelopedTierNeverFlags(t *testing.T) {
	combos := []Input{
		{PriceTier: subscription.PriceTierDeveloped, CardCountry: "US", BillingCountry: "IN", IPCountry: "GB"},
		{PriceTier: subscription.PriceTierDeveloped, CardCountry: "IN", BillingCountry: "IN", IPCountry: "IN"},
		{PriceTier: subscription.PriceTierDeveloped, CardCountry: "US", BillingCountry: "US", IPCountry: "US"},
	}
	for _, in := range combos {
		got := Evaluate(in)
		if got.Flagged {
			t.Errorf("developed tier should never flag; got Flagged=true for %+v", in)
		}
	}
}

func TestEvaluate_MissingIPCountryDoesNotFlag(t *testing.T) {
	in := Input{
		PriceTier:      subscription.PriceTierPPP,
		CardCountry:    "US",
		BillingCountry: "IN",
		IPCountry:      "??",
	}
	got := Evaluate(in)
	if got.Flagged {
		t.Errorf("missing IP country should not flag; got Flagged=true")
	}
	if got.Note != ReasonIPUnknown {
		t.Errorf("Note=%q, want %q", got.Note, ReasonIPUnknown)
	}
}

func TestEvaluate_IsPure(t *testing.T) {
	in := Input{
		PriceTier:      subscription.PriceTierPPP,
		CardCountry:    "US",
		BillingCountry: "IN",
		IPCountry:      "IN",
	}
	d1 := Evaluate(in)
	d2 := Evaluate(in)
	if d1 != d2 {
		t.Errorf("Evaluate is not pure: first=%+v second=%+v", d1, d2)
	}
}
