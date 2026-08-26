// Package-level template registration for billing mail (#381).
//
// Every key here is registered against the shared emailtemplates.Loader at
// boot, exactly as orderdoc and giftcard do. Registration makes a key
// overridable from the operator console; it does not seed a DB row, so
// until someone authors one these embedded fallbacks are what sends.
package email

import (
	"fmt"
	"strings"

	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
)

// bodyMarker is the substitution point in chromeHTML.
const bodyMarker = "<!--BODY-->"

// billingTemplateKeys is the catalog, in the order an operator would
// encounter them across a subscription's life.
var billingTemplateKeys = []TemplateID{
	TemplateTrialNoPMT15,
	TemplateTrialNoPMT10,
	TemplateTrialNoPMT7,
	TemplateTrialNoPMT3,
	TemplateTrialNoPMT1,
	TemplateTrialHasPMT1,
	TemplateTrialStartedBilled,
	TemplatePaymentActionReminder,
	TemplateDunningDay5,
	TemplateDunningDay7,
	TemplateWinBack,
}

// BillingTemplateKeys returns a copy of the catalog. A copy, so a caller
// cannot reorder or truncate the registry it is reading.
func BillingTemplateKeys() []TemplateID {
	out := make([]TemplateID, len(billingTemplateKeys))
	copy(out, billingTemplateKeys)
	return out
}

// RegisterFallbacks binds every billing template's embedded fallback to
// the loader. Call once at boot, before any cron can fire.
//
// Panics if a key is missing content — a programming error that would
// otherwise surface as a silently unsent email at 09:05 UTC.
func RegisterFallbacks(loader *emailtemplates.Loader) {
	for _, key := range billingTemplateKeys {
		subject, ok := billingSubjects[key]
		if !ok {
			panic(fmt.Sprintf("email: no subject registered for template %q", key))
		}
		htmlFragment, ok := billingHTMLFragments[key]
		if !ok {
			panic(fmt.Sprintf("email: no html fragment registered for template %q", key))
		}
		textBody, ok := billingTextFragments[key]
		if !ok {
			panic(fmt.Sprintf("email: no text fragment registered for template %q", key))
		}
		loader.Register(string(key), emailtemplates.EmbeddedFallback{
			Subject:  subject,
			HTMLBody: strings.Replace(chromeHTML, bodyMarker, htmlFragment, 1),
			TextBody: textBody,
		})
	}
}
