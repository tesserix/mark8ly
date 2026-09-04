package pricing

import (
	"fmt"
	"go/format"
	"strings"
)

// RegenerateCatalogCommand is the exact command that produces catalog_data.go
// from this package. It is quoted in the generated file's header and in
// gencatalog_test.go's failure message so a reader never has to guess how to
// fix a stale file.
//
// It writes through -o rather than a stdout redirect (the shape cmd/genpricing
// uses) because the destination is a file in this very package: a shell
// redirect truncates it before `go run` has compiled the package, so the run
// that was meant to rewrite the file cannot build.
const RegenerateCatalogCommand = "cd services/marketplace-api && go run ./internal/billing/pricing/cmd/gencatalog -o internal/billing/pricing/catalog_data.go"

// developedEmitCurrencyOrder is the order developed-tier currencies are
// written into catalog_data.go. It matches the currency list init() reads back
// out when it builds each developed descriptor's Options map; map literal
// order carries no meaning at runtime, so this is purely how the file reads.
var developedEmitCurrencyOrder = []string{"usd", "cad", "gbp", "eur", "aud", "nzd", "sgd"}

// pppEmitCurrencyOrder is the same, for the PPP tier, in the order spec §4.1.1
// lists the six PPP markets.
var pppEmitCurrencyOrder = []string{"inr", "myr", "thb", "php", "idr", "vnd"}

// periodEmitOrder pairs with planOrder (gen.go) to walk plan/period groups.
var periodEmitOrder = []Period{PeriodMonthly, PeriodAnnual}

// planConstName and periodConstName give the Go identifier for each value, so
// the emitted literal reads with the same constants a human would have typed
// rather than with raw strings.
var planConstName = map[Plan]string{
	PlanStarter: "PlanStarter",
	PlanStudio:  "PlanStudio",
	PlanPro:     "PlanPro",
}

var periodConstName = map[Period]string{
	PeriodMonthly: "PeriodMonthly",
	PeriodAnnual:  "PeriodAnnual",
}

// developedGroupComment is the prose that sits above one (plan, period) block
// of developed-market amounts. This is commentary, not billing data — it cites
// the spec figures and, for Pro monthly, shows the +20% derivation behind each
// currency — so it is preserved verbatim here rather than derived, the same
// way gen.go preserves planHeaderComment.
var developedGroupComment = map[Plan]map[Period][]string{
	PlanStarter: {
		PeriodAnnual: {
			"// Spec §4.1: $182, C$239, £144, €163, A$278+GST, NZ$278, S$239",
		},
	},
	PlanStudio: {
		PeriodAnnual: {
			"// Spec §4.1: $470, C$625, £375, €432, A$719+GST, NZ$719, S$623",
		},
	},
	PlanPro: {
		PeriodMonthly: {
			"// Spec §4.1: monthly at +20% premium over annual-equivalent.",
			"// Annual: $1188/yr → $99/mo eq → monthly = $99 × 1.20 = $118.80 → $119",
			"// CAD: C$1619/yr → C$134.92/mo eq → ×1.20 = C$161.90 → C$162",
			"// GBP: £948/yr → £79/mo eq → ×1.20 = £94.80 → £95",
			"// EUR: €1068/yr → €89/mo eq → ×1.20 = €106.80 → €107",
			"// AUD: A$1788/yr → A$149/mo eq → ×1.20 = A$178.80 → A$179",
			"// NZD: NZ$1788/yr → NZ$149/mo eq → ×1.20 = NZ$178.80 → NZ$179",
			"// SGD: S$1548/yr → S$129/mo eq → ×1.20 = S$154.80 → S$155",
		},
		PeriodAnnual: {
			"// Spec §4.1: $1,188/yr, C$1,619/yr, £948/yr, €1,068/yr, A$1,788/yr, NZ$1,788/yr, S$1,548/yr",
		},
	},
}

// pppGroupComment is the same for the PPP tier: a divider naming the group and
// the spec line listing that group's six figures. Preserved verbatim for the
// same reason as developedGroupComment.
var pppGroupComment = map[Plan]map[Period][]string{
	PlanStarter: {
		PeriodMonthly: {
			"// --- Starter monthly PPP ---",
			"// Spec §4.1.1: ₹999, RM59, ฿499, ₱749, Rp199,000, ₫329,000",
		},
		PeriodAnnual: {
			"// --- Starter annual PPP ---",
			"// Spec §4.1.1: ₹9,599, RM569, ฿4,799, ₱7,199, Rp1,919,000, ₫3,169,000",
		},
	},
	PlanStudio: {
		PeriodMonthly: {
			"// --- Studio monthly PPP ---",
			"// Spec §4.1.1: ₹2,499, RM149, ฿1,199, ₱1,899, Rp499,000, ₫799,000",
		},
		PeriodAnnual: {
			"// --- Studio annual PPP ---",
			"// Spec §4.1.1: ₹23,999, RM1,429, ฿11,519, ₱18,239, Rp4,799,000, ₫7,699,000",
		},
	},
	PlanPro: {
		PeriodMonthly: {
			"// --- Pro monthly PPP ---",
			"// Spec §4.1.1 lists only the annual Pro PPP figure for India (₹65,999/yr, ₹5,499/mo eq).",
			"// Monthly Pro PPP is available at +20% premium over annual-equivalent:",
			"//   INR: ₹5,499 × 1.20 = ₹6,598.80 → ₹6,599",
			"//   MYR: RM3,588/12=299 × 1.20 = RM358.80 → RM359",
			"//   THB: ฿28,788/12=2399 × 1.20 = ฿2,878.80 → ฿2,879",
			"//   PHP: ₱45,588/12=3799 × 1.20 = ₱4,558.80 → ₱4,559",
			"//   IDR: Rp11,988,000/12=999,000 × 1.20 = Rp1,198,800 → Rp1,198,800",
			"//   VND: ₫19,788,000/12=1,649,000 × 1.20 = ₫1,978,800 → ₫1,978,800",
		},
		PeriodAnnual: {
			"// --- Pro annual PPP ---",
			"// Spec §4.1.1: ₹65,999/yr, RM3,588/yr, ฿28,788/yr, ₱45,588/yr, Rp11,988,000/yr, ₫19,788,000/yr",
		},
	},
}

// pppWholeUnitCurrencies are the PPP currencies whose amounts carry no
// sub-unit part, and whose rows the hand-written table therefore annotated as
// a whole number of major units rather than with a trailing ".00". Every
// amount in this catalog — these included — is stored ×100; catalog.go's PPP
// section header covers that convention and which currency needs it undone at
// the Stripe boundary.
var pppWholeUnitCurrencies = map[string]bool{"idr": true, "vnd": true}

// GenerateCatalogData renders the full contents of catalog_data.go from the
// two amount tables that file declares. The result is run through go/format,
// so the committed file is gofmt-clean and the byte comparison in
// gencatalog_test.go is stable.
//
// GenerateCatalogDataFromRows renders the same file from a catalog read
// elsewhere — the console — through the same emitters, so both sources
// produce byte-identical output when they agree.
func GenerateCatalogData() (string, error) {
	return generateCatalogData(developedAmounts, pppAmounts)
}

// generateCatalogData is the one emitter both sources go through.
func generateCatalogData(developed map[Plan]map[Period]map[string]Amount, ppp map[pppKey]Amount) (string, error) {
	var b strings.Builder

	b.WriteString("// Code generated by cmd/gencatalog. DO NOT EDIT.\n")
	b.WriteString("//\n")
	b.WriteString("// Regenerate with:\n")
	b.WriteString("//   " + RegenerateCatalogCommand + "\n")
	b.WriteString("//\n")
	b.WriteString("// These are the amount tables catalog.go turns into PriceDescriptors: the\n")
	b.WriteString("// fallback the platform console's billing read routes serve from when the\n")
	b.WriteString("// console catalog is unconfigured or unreachable. Adding or changing an\n")
	b.WriteString("// amount by hand here is how the two sources drift; change it in the\n")
	b.WriteString("// console and regenerate.\n")
	b.WriteString("//\n")
	b.WriteString("// Types, lookup-key derivation and the query helpers are hand-written and\n")
	b.WriteString("// live in catalog.go, alongside the completeness this file has to keep.\n")
	b.WriteString("\n")
	b.WriteString("package pricing\n")
	b.WriteString("\n")

	writeDevelopedTable(&b, developed)
	b.WriteString("\n")
	writePPPTable(&b, ppp)

	formatted, err := format.Source([]byte(b.String()))
	if err != nil {
		return "", fmt.Errorf("gofmt of generated catalog data: %w", err)
	}
	return string(formatted), nil
}

// writeDevelopedTable emits the developedAmounts literal, one nested block per
// plan and period, currencies in developedEmitCurrencyOrder.
func writeDevelopedTable(b *strings.Builder, developed map[Plan]map[Period]map[string]Amount) {
	b.WriteString("// developedAmounts maps (plan, period) → per-currency Amount.\n")
	b.WriteString("// Key: lowercase ISO 4217. Baseline currency is always \"usd\".\n")
	b.WriteString("var developedAmounts = map[Plan]map[Period]map[string]Amount{\n")
	for _, plan := range planOrder {
		byPeriod, ok := developed[plan]
		if !ok {
			continue
		}
		b.WriteString("\t" + planConstName[plan] + ": {\n")
		for _, period := range periodEmitOrder {
			byCurrency, ok := byPeriod[period]
			if !ok {
				continue
			}
			b.WriteString("\t\t" + periodConstName[period] + ": {\n")
			for _, line := range developedGroupComment[plan][period] {
				b.WriteString("\t\t\t" + line + "\n")
			}
			for _, currency := range developedEmitCurrencyOrder {
				amt, ok := byCurrency[currency]
				if !ok {
					continue
				}
				b.WriteString(fmt.Sprintf("\t\t\t%q: %s,\n", currency, amountLiteral(amt)))
			}
			b.WriteString("\t\t},\n")
		}
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n")
}

// writePPPTable emits the pppAmounts literal, grouped by (plan, period) with
// the group's prose above it and each row annotated with the real-world figure
// its stored value stands for.
func writePPPTable(b *strings.Builder, ppp map[pppKey]Amount) {
	b.WriteString("// pppAmounts holds every PPP price entry keyed by (plan, period, currency).\n")
	b.WriteString("var pppAmounts = map[pppKey]Amount{\n")
	first := true
	for _, plan := range planOrder {
		for _, period := range periodEmitOrder {
			rows := make([]string, 0, len(pppEmitCurrencyOrder))
			for _, currency := range pppEmitCurrencyOrder {
				amt, ok := ppp[pppKey{plan: plan, period: period, currency: currency}]
				if !ok {
					continue
				}
				rows = append(rows, fmt.Sprintf("\t{%s, %s, %q}: %s, // %s\n",
					planConstName[plan], periodConstName[period], currency,
					amountLiteral(amt), humanAmount(amt)))
			}
			if len(rows) == 0 {
				continue
			}
			if !first {
				b.WriteString("\n")
			}
			first = false
			for _, line := range pppGroupComment[plan][period] {
				b.WriteString("\t" + line + "\n")
			}
			for _, r := range rows {
				b.WriteString(r)
			}
		}
	}
	b.WriteString("}\n")
}

// amountLiteral renders one Amount as the composite literal it is written as
// in catalog_data.go, omitting TaxBehavior when it is the Stripe default.
func amountLiteral(a Amount) string {
	lit := fmt.Sprintf("{Currency: %q, UnitAmountMinor: %d", a.Currency, a.UnitAmountMinor)
	if a.TaxBehavior != "" {
		lit += fmt.Sprintf(", TaxBehavior: %q", a.TaxBehavior)
	}
	return lit + "}"
}

// humanAmount renders the real-world figure a stored UnitAmountMinor stands
// for, as the trailing annotation on a PPP row: "9,599.00 INR", or
// "199,000 IDR (stored ×100)" for the whole-unit currencies.
func humanAmount(a Amount) string {
	major := groupDigits(a.UnitAmountMinor / 100)
	code := strings.ToUpper(a.Currency)
	if pppWholeUnitCurrencies[a.Currency] {
		return major + " " + code + " (stored ×100)"
	}
	return fmt.Sprintf("%s.%02d %s", major, a.UnitAmountMinor%100, code)
}

// groupDigits renders a non-negative integer with comma thousands separators,
// matching how the hand-written rows read (9599 → "9,599").
func groupDigits(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}
