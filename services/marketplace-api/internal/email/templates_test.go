package email_test

import (
	"context"
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
)

// sampleVars covers every field any billing template references, so one
// map renders them all. Extra keys are harmless.
func sampleVars() map[string]any {
	return map[string]any{
		"store_id":           "11111111-2222-3333-4444-555555555555",
		"tenant_id":          "66666666-7777-8888-9999-000000000000",
		"store_name":         "Acme Supply Co",
		"day":                5,
		"offset":             "t_minus_7",
		"days_remaining":     7,
		"has_payment_method": false,
		"plan":               "growth",
		"period":             "monthly",
		"promo":              "20%-off-6-months",
		"hosted_invoice_url": "https://invoice.stripe.com/i/test",
	}
}

func TestRegisterFallbacks_EveryKeyRenders(t *testing.T) {
	loader := emailtemplates.NewLoader(nil)
	email.RegisterFallbacks(loader)

	keys := email.BillingTemplateKeys()
	if len(keys) != 14 {
		t.Fatalf("BillingTemplateKeys() has %d keys, want 14", len(keys))
	}

	for _, key := range keys {
		t.Run(string(key), func(t *testing.T) {
			r, err := loader.Render(context.Background(), string(key), sampleVars())
			if err != nil {
				t.Fatalf("Render(%s) failed: %v", key, err)
			}
			if strings.TrimSpace(r.Subject) == "" {
				t.Error("empty subject")
			}
			if strings.TrimSpace(r.TextBody) == "" {
				t.Error("empty text body")
			}
			if !strings.Contains(r.HTMLBody, "<!doctype html>") {
				t.Error("html body is not a full document — chrome not applied")
			}
			if strings.Contains(r.HTMLBody, "<!--BODY-->") {
				t.Error("chrome placeholder was not substituted")
			}
			// Every template addresses the merchant by store name.
			if !strings.Contains(r.HTMLBody, "Acme Supply Co") {
				t.Error("html body does not reference store_name")
			}
			// No unresolved template actions should survive rendering.
			if strings.Contains(r.HTMLBody, "{{") || strings.Contains(r.TextBody, "{{") {
				t.Error("unrendered template action left in output")
			}
			// A misspelled field renders as the literal "<no value>" rather
			// than failing, so absence of "{{" is not enough — assert the
			// rendered output does not contain it. html/template escapes the
			// angle brackets, so check both forms.
			for _, body := range []string{r.HTMLBody, r.TextBody, r.Subject} {
				if strings.Contains(body, "<no value>") || strings.Contains(body, "&lt;no value&gt;") {
					t.Errorf("template rendered a missing variable as <no value>: %s", body)
				}
			}
		})
	}
}

func TestRegisterFallbacks_HostedInvoiceURLIsOptional(t *testing.T) {
	loader := emailtemplates.NewLoader(nil)
	email.RegisterFallbacks(loader)

	vars := sampleVars()
	vars["hosted_invoice_url"] = ""

	r, err := loader.Render(context.Background(), string(email.TemplateDunningDay5), vars)
	if err != nil {
		t.Fatalf("Render without invoice url failed: %v", err)
	}
	if strings.Contains(r.HTMLBody, "href=\"\"") {
		t.Error("emitted an empty href instead of omitting the CTA")
	}
}

func TestBillingTemplateKeys_IncludesWinBack(t *testing.T) {
	var found bool
	for _, k := range email.BillingTemplateKeys() {
		if k == email.TemplateWinBack {
			found = true
		}
	}
	if !found {
		t.Error("win_back_day30 missing from the billing catalog")
	}
}
