package arbitrage

import (
	"fmt"
	"strings"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// Input is the full triangulation surface. All country codes are ISO-3166-1
// alpha-2 after Normalize; "??" means missing.
type Input struct {
	PriceTier      subscription.PriceTier
	CardCountry    string
	BillingCountry string
	IPCountry      string
}

// Decision is what Evaluate returns. Flagged = write the audit row. Note is a
// non-flag observability hint (e.g. IP signal missing); MismatchReason is the
// human-readable rationale for a flag.
type Decision struct {
	Flagged        bool
	MismatchReason string
	Note           string
}

const (
	// ReasonIPUnknown — the IP country was "??" so we didn't have enough
	// signal to flag; billing-ops dashboards track these separately.
	ReasonIPUnknown = "ip_country_unknown"
)

// Evaluate is a pure function: no DB, no clock, no external calls. Given the
// three country signals + the resolved price tier, it decides whether to flag.
//
// Rules (§18.8):
//   - Developed-tier subscriptions are never flagged (no arbitrage incentive).
//   - PPP-tier subscriptions are flagged iff card_country OR ip_country points
//     at a developed market. billing_country alone is NOT sufficient to flag,
//     because a legitimate merchant may list a registered office in an
//     emerging market while paying via a developed-country corporate card —
//     that's exactly the kind of case billing-ops decides, not the code.
//   - Missing IP country ("??") downgrades to a Note; we don't flag on card
//     alone because travelers and dual-citizens would swamp the queue.
func Evaluate(in Input) Decision {
	if in.PriceTier != subscription.PriceTierPPP {
		return Decision{}
	}
	card := NormalizeCountry(in.CardCountry)
	ip := NormalizeCountry(in.IPCountry)

	if ip == "??" {
		return Decision{Note: ReasonIPUnknown}
	}

	reasons := make([]string, 0, 2)
	if IsDevelopedMarket(card) {
		reasons = append(reasons, fmt.Sprintf("card_country=%s (developed)", card))
	}
	if IsDevelopedMarket(ip) {
		reasons = append(reasons, fmt.Sprintf("ip_country=%s (developed)", ip))
	}
	if len(reasons) == 0 {
		return Decision{}
	}
	return Decision{
		Flagged:        true,
		MismatchReason: "PPP tier with " + strings.Join(reasons, "; "),
	}
}
