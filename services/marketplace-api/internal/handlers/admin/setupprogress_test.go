package admin

import "testing"

// A brand-new store (only the store row + prefilled settings exist) must
// start at exactly 20% — the "head start" the dashboard promises. If a
// step is ever added or removed, this pins the recalibration.
func TestProgressFromNewStore(t *testing.T) {
	p := progressFrom(SetupChecklist{HasStore: true})

	if p.CompletedSteps != 2 {
		t.Fatalf("new store completed_steps = %d, want 2 (store created + prefilled settings)", p.CompletedSteps)
	}
	if p.TotalSteps != totalSetupSteps {
		t.Fatalf("total_steps = %d, want %d", p.TotalSteps, totalSetupSteps)
	}
	if p.Percent != 20 {
		t.Fatalf("new store percent = %d, want 20", p.Percent)
	}
}

func TestProgressFromAllComplete(t *testing.T) {
	p := progressFrom(SetupChecklist{
		HasStore:           true,
		HasBrandAssets:     true,
		HasProduct:         true,
		HasStorefrontTheme: true,
		HasPaymentProvider: true,
		HasShippingCarrier: true,
		HasReturnPolicy:    true,
		HasCustomDomain:    true,
		HasFirstOrder:      true,
	})

	if p.CompletedSteps != p.TotalSteps {
		t.Fatalf("completed_steps = %d, want %d", p.CompletedSteps, p.TotalSteps)
	}
	if p.Percent != 100 {
		t.Fatalf("percent = %d, want 100", p.Percent)
	}
}
