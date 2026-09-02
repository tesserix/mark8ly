// Package plangate — matrix.go encodes the v2.3 feature matrix (spec §9)
// as the single source of truth for plan × feature limits. Every feature
// constant and sentinel value used by callers lives here.
package plangate

import (
	"time"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// Feature identifies a matrix row. String values are stable and surface in
// API responses (e.g. AllFeatureLimits), so renaming one is a breaking
// change for the frontend.
type Feature string

const (
	// Limits.
	FeatureStores                 Feature = "stores"
	FeatureImagesPerProduct       Feature = "images_per_product"
	FeatureAuditRetentionDays     Feature = "audit_retention_days"
	FeatureCampaignEmailsPerMonth Feature = "campaign_emails_per_month"
	FeatureTransactionalEmails    Feature = "transactional_emails"
	// FeatureWebhookSubscriptions caps outbound webhook subscriptions per
	// STORE (not per tenant) — #586. Dispatch fan-out is
	// `outbox rows × matching subscriptions`, so an unbounded count turns
	// one order.placed into an unbounded number of delivery rows and
	// outbound HTTP attempts against a shared db-f1-micro. Enforced at
	// creation only, mirroring FeatureImagesPerProduct: a downgrade does
	// not delete a merchant's existing subscriptions.
	FeatureWebhookSubscriptions Feature = "webhook_subscriptions"

	// Storefront.
	FeatureCustomDomain     Feature = "custom_domain"
	FeatureFullColorPalette Feature = "full_color_palette"
	FeatureAnnouncementBar  Feature = "announcement_bar"
	FeatureRemovePoweredBy  Feature = "remove_powered_by"
	FeatureCustomCSS        Feature = "custom_css"
	// FeatureCustomCodeInjection — reserved for the future head/body
	// script-injection surface. No handler accepts arbitrary <script>
	// payloads today, so the gate is a forward-compat hedge. When the
	// surface ships, add plangate.RequireFeature(FeatureCustomCodeInjection)
	// at the write handler AND ensure the render path CSPs/escapes the
	// stored value. Pro-only in the matrix.
	FeatureCustomCodeInjection Feature = "custom_code_injection"
	FeatureWhiteLabelApp       Feature = "white_label_app" // always Disabled in matrix; gated by add-on

	// Platform.
	FeatureCSVImportExport Feature = "csv_import_export"
	FeatureShippingLabels  Feature = "shipping_labels"
	FeatureReturns         Feature = "returns"
	FeatureReviews         Feature = "reviews"
	FeatureTickets         Feature = "tickets"
	FeatureGiftCards       Feature = "gift_cards"
	FeatureReadAPI         Feature = "read_api"
	FeatureFullAPI         Feature = "full_read_write_api"
	FeatureSSO             Feature = "sso"
	FeatureUptimeSLA       Feature = "uptime_sla"

	// Support.
	FeatureStandardEmailSupport Feature = "standard_email_support"
	FeaturePriorityEmailSupport Feature = "priority_email_support"
	FeatureNamedCSM             Feature = "named_csm"
)

// Sentinel values stored in the matrix. 0 (Disabled) is the zero value so an
// unset cell is implicitly disabled. Unlimited and Negotiated use negative
// ints so Limit() callers can distinguish "off" from "no cap" without an
// extra bool return.
const (
	Disabled   = 0
	Unlimited  = -1
	Negotiated = -2 // frontend renders "contact sales"
)

// allFeatures is the canonical ordered list used by AllFeatures() and the
// parity test. Keep it in sync with the const block above — the parity
// test asserts the length matches the number of constants.
var allFeatures = []Feature{
	FeatureStores,
	FeatureImagesPerProduct,
	FeatureAuditRetentionDays,
	FeatureCampaignEmailsPerMonth,
	FeatureTransactionalEmails,
	FeatureWebhookSubscriptions,
	FeatureCustomDomain,
	FeatureFullColorPalette,
	FeatureAnnouncementBar,
	FeatureRemovePoweredBy,
	FeatureCustomCSS,
	FeatureCustomCodeInjection,
	FeatureWhiteLabelApp,
	FeatureCSVImportExport,
	FeatureShippingLabels,
	FeatureReturns,
	FeatureReviews,
	FeatureTickets,
	FeatureGiftCards,
	FeatureReadAPI,
	FeatureFullAPI,
	FeatureSSO,
	FeatureUptimeSLA,
	FeatureStandardEmailSupport,
	FeaturePriorityEmailSupport,
	FeatureNamedCSM,
}

// AllFeatures returns a defensive copy of the canonical feature list. The
// copy protects callers from accidentally mutating the package-level slice.
func AllFeatures() []Feature {
	out := make([]Feature, len(allFeatures))
	copy(out, allFeatures)
	return out
}

type planLimits map[Feature]int

// featureMatrix encodes spec §9 verbatim. Cells that diverge from the raw
// spec text are annotated inline. Shape is map[Plan]map[Feature]int so
// reads are two map lookups (both constant-time) rather than an iteration
// over features.
//
// PlanMarketplace is intentionally absent — it's a hidden platform tier;
// platform routes bypass plangate entirely.
var featureMatrix = map[subscription.SubscriptionPlan]planLimits{
	subscription.PlanTrial: {
		FeatureStores:                 1,
		FeatureImagesPerProduct:       25,
		FeatureAuditRetentionDays:     90,
		FeatureCampaignEmailsPerMonth: 5_000,
		FeatureTransactionalEmails:    Unlimited, // ∞ with 100k/mo fair-use (§9)
		FeatureWebhookSubscriptions:   5,

		FeatureCustomDomain:        1,
		FeatureFullColorPalette:    1,
		FeatureAnnouncementBar:     1,
		FeatureRemovePoweredBy:     1,
		FeatureCustomCSS:           Disabled,
		FeatureCustomCodeInjection: Disabled,
		FeatureWhiteLabelApp:       Disabled, // add-on only (§9 Pro+App)

		FeatureCSVImportExport: 1,
		FeatureShippingLabels:  1,
		FeatureReturns:         1,
		FeatureReviews:         1,
		FeatureTickets:         1,
		FeatureGiftCards:       1,
		// #585 (follow-up): widened from Disabled for the same reason
		// Starter was. Webhooks ship on every plan and deliver only an
		// event name and an aggregate id, so a Trial merchant evaluating
		// an integration could receive order.placed and have no way to
		// resolve it — and would reasonably conclude webhooks are broken
		// during the exact window we are trying to convert them in.
		// The read API is consequently no longer a paid differentiator
		// on any tier; FeatureFullAPI (write) remains Pro-only.
		FeatureReadAPI:   1,
		FeatureFullAPI:   Disabled,
		FeatureSSO:       Disabled,
		FeatureUptimeSLA: Disabled,

		FeatureStandardEmailSupport: 1,
		FeaturePriorityEmailSupport: Disabled,
		FeatureNamedCSM:             Disabled,
	},
	subscription.PlanStarter: {
		FeatureStores:                 2,
		FeatureImagesPerProduct:       25,
		FeatureAuditRetentionDays:     90,
		FeatureCampaignEmailsPerMonth: 15_000,
		FeatureTransactionalEmails:    Unlimited,
		FeatureWebhookSubscriptions:   10,

		FeatureCustomDomain:        1,
		FeatureFullColorPalette:    1,
		FeatureAnnouncementBar:     1,
		FeatureRemovePoweredBy:     1,
		FeatureCustomCSS:           Disabled,
		FeatureCustomCodeInjection: Disabled,
		FeatureWhiteLabelApp:       Disabled,

		FeatureCSVImportExport: 1,
		FeatureShippingLabels:  1,
		FeatureReturns:         1,
		FeatureReviews:         1,
		FeatureTickets:         1,
		FeatureGiftCards:       1,
		// #585: widened from Disabled. Outbound webhooks (#562) ship on
		// every plan and use a notify-and-fetch payload — the delivery
		// body carries only an event name and an aggregate id, so the
		// merchant MUST call the read API to resolve it. Leaving this
		// Disabled on Starter shipped "a doorbell with the door locked":
		// a Starter merchant could register a webhook, receive
		// order.placed, and have no supported way to act on it.
		// Diverges from spec §9, deliberately — see #585 for the
		// trade-off (the read API stops being a Studio differentiator).
		FeatureReadAPI:   1,
		FeatureFullAPI:   Disabled,
		FeatureSSO:       Disabled,
		FeatureUptimeSLA: Disabled,

		FeatureStandardEmailSupport: 1,
		FeaturePriorityEmailSupport: Disabled,
		FeatureNamedCSM:             Disabled,
	},
	subscription.PlanStudio: {
		FeatureStores:                 5,
		FeatureImagesPerProduct:       50,
		FeatureAuditRetentionDays:     365,
		FeatureCampaignEmailsPerMonth: 50_000,
		FeatureTransactionalEmails:    Unlimited,
		FeatureWebhookSubscriptions:   25,

		FeatureCustomDomain:        1,
		FeatureFullColorPalette:    1,
		FeatureAnnouncementBar:     1,
		FeatureRemovePoweredBy:     1,
		FeatureCustomCSS:           1,
		FeatureCustomCodeInjection: Disabled,
		FeatureWhiteLabelApp:       Disabled,

		FeatureCSVImportExport: 1,
		FeatureShippingLabels:  1,
		FeatureReturns:         1,
		FeatureReviews:         1,
		FeatureTickets:         1,
		FeatureGiftCards:       1,
		FeatureReadAPI:         1,
		FeatureFullAPI:         Disabled,
		FeatureSSO:             Disabled,
		FeatureUptimeSLA:       Disabled,

		FeatureStandardEmailSupport: 1,
		FeaturePriorityEmailSupport: Disabled,
		FeatureNamedCSM:             Disabled,
	},
	subscription.PlanPro: {
		FeatureStores:                 10,
		FeatureImagesPerProduct:       Unlimited,
		FeatureAuditRetentionDays:     Unlimited, // "Forever" (§9)
		FeatureCampaignEmailsPerMonth: Negotiated,
		FeatureTransactionalEmails:    Negotiated, // "Negotiated ceiling" (§9)
		// 100, deliberately under the ~132 subscriptions that overflowed
		// Postgres's 65535-parameter limit in the original fan-out outage
		// (#562 chunked the insert; this keeps even the top tier away from
		// the region that broke it).
		FeatureWebhookSubscriptions: 100,

		FeatureCustomDomain:        1,
		FeatureFullColorPalette:    1,
		FeatureAnnouncementBar:     1,
		FeatureRemovePoweredBy:     1,
		FeatureCustomCSS:           1,
		FeatureCustomCodeInjection: 1,
		FeatureWhiteLabelApp:       Disabled, // add-on boolean; see WhiteLabelAppEnabled()

		FeatureCSVImportExport: 1,
		FeatureShippingLabels:  1,
		FeatureReturns:         1,
		FeatureReviews:         1,
		FeatureTickets:         1,
		FeatureGiftCards:       1,
		FeatureReadAPI:         1,
		FeatureFullAPI:         1,
		FeatureSSO:             1,
		FeatureUptimeSLA:       Disabled, // add-on only (Pro + App)

		// §9: Pro drops standard support in favor of priority. CSM is add-on only.
		FeatureStandardEmailSupport: Disabled,
		FeaturePriorityEmailSupport: 1,
		FeatureNamedCSM:             Disabled,
	},
	// PlanMarketplace intentionally omitted — platform tier bypasses plangate.
}

// IsAllowed reports whether the feature is enabled (non-Disabled) for p.
// Unknown plans and unknown features both fail closed.
func IsAllowed(p subscription.SubscriptionPlan, f Feature) bool {
	limits, ok := featureMatrix[p]
	if !ok {
		return false
	}
	v, ok := limits[f]
	if !ok {
		return false
	}
	return v != Disabled
}

// Limit returns the numeric limit (or a sentinel: Disabled / Unlimited /
// Negotiated) for (p, f). Unknown (p, f) pairs return Disabled (0).
func Limit(p subscription.SubscriptionPlan, f Feature) int {
	limits, ok := featureMatrix[p]
	if !ok {
		return Disabled
	}
	return limits[f]
}

// WhiteLabelAppEnabled is the single source of truth for white-label app
// eligibility: the plan must be Pro AND the tenant must have purchased the
// add-on. The matrix deliberately keeps FeatureWhiteLabelApp disabled for
// every plan so accidental IsAllowed() calls fail closed.
func WhiteLabelAppEnabled(p subscription.SubscriptionPlan, hasAddOn bool) bool {
	return p == subscription.PlanPro && hasAddOn
}

// AllFeatureLimits returns a JSON-ready snapshot of every feature's value
// for p. Every Feature returned by AllFeatures() is represented in the
// output — unset cells surface as Disabled (0) — so frontends can iterate
// without branching on key presence.
func AllFeatureLimits(p subscription.SubscriptionPlan) map[string]int {
	limits := featureMatrix[p]
	out := make(map[string]int, len(allFeatures))
	for _, f := range allFeatures {
		if v, ok := limits[f]; ok {
			out[string(f)] = v
		} else {
			out[string(f)] = Disabled
		}
	}
	return out
}

// MinPlanForFeature returns the lowest plan that enables f. Falls back to
// PlanPro when no plan enables the feature — intentionally conservative so
// the 403 "upgrade to X" hint never points at a plan that can't satisfy it.
func MinPlanForFeature(f Feature) subscription.SubscriptionPlan {
	order := []subscription.SubscriptionPlan{
		subscription.PlanTrial,
		subscription.PlanStarter,
		subscription.PlanStudio,
		subscription.PlanPro,
	}
	for _, p := range order {
		if IsAllowed(p, f) {
			return p
		}
	}
	return subscription.PlanPro
}

// ImagesAllowed returns the image-per-product cap for a product, applying the
// Studio→Starter grandfathering rule from §11. Products created before a
// Studio→Starter change keep the Studio cap (50); products created on or
// after the change date use the Starter cap (25).
//
// Semantics:
//   - If plan is Studio or higher, or Pro: returns the matrix limit for plan (no grandfathering).
//   - If plan is Starter/Trial and lastPlanChangeAt is non-nil and productCreatedAt is
//     strictly before the change timestamp: returns 50 (grandfathered Studio cap).
//   - Otherwise: returns the matrix limit for plan (Starter=25, Trial=25).
//
// lastPlanChangeAt is the subscription.LastPlanChangeAt field, which records
// the most recent plan change. This is the single grandfathering anchor —
// products older than it inherit the previous tier's cap.
func ImagesAllowed(plan subscription.SubscriptionPlan, productCreatedAt time.Time, lastPlanChangeAt *time.Time) int {
	// Non-downgraded plans: straightforward matrix lookup.
	if plan == subscription.PlanStudio || plan == subscription.PlanPro {
		return Limit(plan, FeatureImagesPerProduct)
	}

	// On Starter/Trial, check whether the product predates the plan change.
	if lastPlanChangeAt != nil && productCreatedAt.Before(*lastPlanChangeAt) {
		// Grandfathered at the previous tier's Studio cap.
		return Limit(subscription.PlanStudio, FeatureImagesPerProduct)
	}
	return Limit(plan, FeatureImagesPerProduct)
}
