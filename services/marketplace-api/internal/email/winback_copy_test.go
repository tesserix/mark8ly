package email_test

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
)

// discountClaims are the shapes a reader would take as a promise of money
// off. The offer-less win-back must contain none of them in any of its three
// parts — that is the whole of #727: the day-30 mail quantified an offer
// nothing could honour.
var discountClaims = []string{"%", "discount", "off your", "off for", "promo", "code"}

// htmlTag strips markup so the claim scan reads what a MERCHANT reads. The
// shared chrome is full of layout attributes — width="100%" among them — and
// scanning raw HTML would fail on those while telling you nothing about the
// copy.
var htmlTag = regexp.MustCompile(`<[^>]*>`)

func visibleText(html string) string { return htmlTag.ReplaceAllString(html, " ") }

func renderWinBack(t *testing.T, key email.TemplateID, vars map[string]any) emailtemplates.Rendered {
	t.Helper()
	loader := emailtemplates.NewLoader(nil)
	email.RegisterFallbacks(loader)
	r, err := loader.Render(context.Background(), string(key), vars)
	if err != nil {
		t.Fatalf("Render(%s): %v", key, err)
	}
	return r
}

func TestWinBackNoOffer_MakesNoDiscountClaim(t *testing.T) {
	r := renderWinBack(t, email.TemplateWinBackNoOffer, map[string]any{
		"store_name": "Acme Supply Co",
	})

	for _, part := range []struct{ name, body string }{
		{"subject", r.Subject},
		{"html", visibleText(r.HTMLBody)},
		{"text", r.TextBody},
	} {
		lower := strings.ToLower(part.body)
		for _, claim := range discountClaims {
			if strings.Contains(lower, claim) {
				t.Errorf("%s contains %q — the offer-less win-back must promise nothing: %s",
					part.name, claim, part.body)
			}
		}
	}
}

// The offer-less variant is still a useful email, not an empty one: it has to
// tell a merchant a month past expiry that their catalogue is intact.
func TestWinBackNoOffer_StillSaysTheStoreIsIntact(t *testing.T) {
	r := renderWinBack(t, email.TemplateWinBackNoOffer, map[string]any{
		"store_name": "Acme Supply Co",
	})
	for _, want := range []string{"Acme Supply Co", "Nothing has been deleted"} {
		if !strings.Contains(r.TextBody, want) {
			t.Errorf("text body does not mention %q: %s", want, r.TextBody)
		}
	}
}

// The offer variant's numbers come from template DATA, not from prose. A
// console row that says 15% for 3 months must produce an email that says 15%
// for 3 months — the #727 defect was a hardcoded "20% off your first six
// months" that no data could contradict.
func TestWinBackOffer_QuotesTheDataNotAHardcodedNumber(t *testing.T) {
	r := renderWinBack(t, email.TemplateWinBack, map[string]any{
		"store_name":      "Acme Supply Co",
		"promo_code":      "COMEBACK15OFF3MONTHS",
		"percent_off":     "15",
		"duration_months": 3,
	})

	for _, part := range []string{r.Subject, r.HTMLBody, r.TextBody} {
		if strings.Contains(part, "20%") || strings.Contains(strings.ToLower(part), "six months") {
			t.Errorf("a hardcoded 20%%/six-months claim survived the data: %s", part)
		}
	}
	for _, want := range []string{"COMEBACK15OFF3MONTHS", "15%", "3 months"} {
		if !strings.Contains(r.TextBody, want) {
			t.Errorf("text body does not carry %q: %s", want, r.TextBody)
		}
	}
	if !strings.Contains(r.Subject, "15%") {
		t.Errorf("subject does not carry the discount from data: %s", r.Subject)
	}
}
