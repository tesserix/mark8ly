package pricing

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// pppCurrenciesFromCatalog is the catalog's own answer, so these tests cannot
// drift from the prices actually published to Stripe.
func pppCurrenciesFromCatalog() []string {
	seen := map[string]struct{}{}
	for _, d := range AllDescriptors() {
		if d.Tier == TierPPP {
			seen[d.Currency] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

func TestTierForCurrency_EveryPPPCurrencyResolvesPPP(t *testing.T) {
	ppp := pppCurrenciesFromCatalog()
	if len(ppp) == 0 {
		t.Fatal("catalog exposes no PPP currencies; the fixture is wrong, not the code")
	}
	for _, c := range ppp {
		if got := TierForCurrency(c); got != TierPPP {
			t.Errorf("TierForCurrency(%q) = %q, want %q — a store billed in a "+
				"currency the catalog prices at PPP would be charged the "+
				"developed price", c, got, TierPPP)
		}
		if got := TierForCurrency(strings.ToUpper(c)); got != TierPPP {
			t.Errorf("TierForCurrency(%q) = %q, want %q (case must not matter: "+
				"billing_currency is lowercased on write but callers may not be)",
				strings.ToUpper(c), got, TierPPP)
		}
	}
}

func TestTierForCurrency_DevelopedAndUnknownResolveDeveloped(t *testing.T) {
	// The seven currency_options carried on the developed Price object.
	for _, c := range []string{"usd", "cad", "gbp", "eur", "aud", "nzd", "sgd"} {
		if got := TierForCurrency(c); got != TierDeveloped {
			t.Errorf("TierForCurrency(%q) = %q, want %q", c, got, TierDeveloped)
		}
	}
	// Unknown and empty fall back to the tier that has a real price for them.
	for _, c := range []string{"", "  ", "zzz", "jpy"} {
		if got := TierForCurrency(c); got != TierDeveloped {
			t.Errorf("TierForCurrency(%q) = %q, want %q", c, got, TierDeveloped)
		}
	}
}

// TestBackfillMigration_CurrencyListMatchesCatalog guards the one place this
// change restates the catalog instead of asking it: migration 000138 cannot
// call Go, so it hardcodes the PPP currency list. If a PPP currency is later
// added to catalog_data.go, that SQL silently stops covering it.
func TestBackfillMigration_CurrencyListMatchesCatalog(t *testing.T) {
	path := filepath.Join("..", "..", "..", "migrations",
		"000138_backfill_price_tier_from_billing_currency.up.sql")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	re := regexp.MustCompile(`(?s)IN \(([^)]*)\)`)
	m := re.FindSubmatch(body)
	if m == nil {
		t.Fatal("no IN (...) currency list found in the migration")
	}
	var inSQL []string
	for _, tok := range strings.Split(string(m[1]), ",") {
		if c := strings.Trim(strings.TrimSpace(tok), "'"); c != "" {
			inSQL = append(inSQL, c)
		}
	}
	sort.Strings(inSQL)
	want := pppCurrenciesFromCatalog()
	if strings.Join(inSQL, ",") != strings.Join(want, ",") {
		t.Errorf("migration 000138 backfills %v but the catalog prices PPP in %v; "+
			"stores billed in the difference keep the developed tier and are "+
			"charged the USD baseline", inSQL, want)
	}
}
