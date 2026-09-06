package email

// templates_lint_test.go — catches a template variable that was misspelled
// or renamed. Production HTML renders through html/template, which prints a
// missing map key as an empty string rather than "<no value>" — silent, and
// invisible to an output-based assertion. Parsing the same source with
// text/template surfaces it. This lints the fragment source only; nothing
// here affects what is actually sent.

import (
	"bytes"
	"strings"
	"testing"
	texttpl "text/template"
)

func TestBillingTemplates_NoMissingVariables(t *testing.T) {
	vars := map[string]any{
		"store_id":           "11111111-2222-3333-4444-555555555555",
		"tenant_id":          "66666666-7777-8888-9999-000000000000",
		"store_name":         "Acme Supply Co",
		"day":                5,
		"offset":             "t_minus_7",
		"days_remaining":     7,
		"has_payment_method": false,
		"plan":               "growth",
		"period":             "monthly",
		"promo_code":         "WINBACK20OFF6MONTHS",
		"percent_off":        "20",
		"duration_months":    6,
		"hosted_invoice_url": "https://invoice.stripe.com/i/test",
	}

	for _, key := range billingTemplateKeys {
		for _, part := range []struct {
			name string
			src  string
		}{
			{"subject", billingSubjects[key]},
			{"html", billingHTMLFragments[key]},
			{"text", billingTextFragments[key]},
		} {
			tpl, err := texttpl.New(string(key) + ":" + part.name).Parse(part.src)
			if err != nil {
				t.Errorf("%s/%s: parse: %v", key, part.name, err)
				continue
			}
			var buf bytes.Buffer
			if err := tpl.Execute(&buf, vars); err != nil {
				t.Errorf("%s/%s: execute: %v", key, part.name, err)
				continue
			}
			if strings.Contains(buf.String(), "<no value>") {
				t.Errorf("%s/%s references a variable not in the caller's data map", key, part.name)
			}
		}
	}
}
