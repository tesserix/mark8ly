// services/marketplace-api/internal/handlers/platformadmin/lifecycle_reason_codes.go
package platformadmin

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ReasonCode pairs a wire code with the words an operator reads.
//
// The code is what crosses the wire, lands in an audit row and is matched
// exactly by the validators in this package. The label is presentation and
// may be reworded freely; renaming a code may not.
type ReasonCode struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

// LifecycleReasonCodesResponse is contract §8.8's body.
//
// A map rather than four named fields: §8.8 requires `suspend` and
// `unsuspend` of every implementer and permits more, and a product's set of
// consequential verbs is its own. Adding one here should not need a struct
// field, and a console reading it should not need a new build to see one.
type LifecycleReasonCodesResponse struct {
	Data map[string][]ReasonCode `json:"data"`
}

// reasonCodeLabels supplies the human wording for each code, per verb.
//
// **Per verb, not global, and that is the whole reason this is a nested map.**
// `operator_error` means "suspended in error" on unsuspend, "correcting a
// mistaken earlier extension" on trial extension, and "a tenant created in
// error" on purge. `fraud` and `legal` likewise differ between suspending a
// tenant and destroying one. A single flat code→label map would have quietly
// shown an operator the wrong sentence next to the right code, which is worse
// than showing them the bare code.
//
// The `[]string` code lists remain the single authority on MEMBERSHIP and
// ORDER — this map is consulted, never enumerated. That keeps the validators
// (`isKnownReasonCode`) and the `allowed` array in the §4.4 error bodies
// exactly as they were: publishing the vocabulary must not become a chance to
// change what the vocabulary is.
//
// TestReasonCodeLabelsCoverEveryCode fails on a code with no label and on a
// label with no code, so the two cannot drift apart.
var reasonCodeLabels = map[string]map[string]string{
	"suspend": {
		"abuse":         "Abuse — abusive content or behaviour",
		"fraud":         "Fraud — suspected fraudulent transactions or identity",
		"non_payment":   "Non-payment — dunning exhausted",
		"legal":         "Legal — legal or regulatory demand",
		"tos_violation": "Terms breach — not covered by abuse or fraud",
		"security":      "Security — compromised account or active incident",
		"voluntary":     "Voluntary — the merchant asked for a pause",
	},
	"unsuspend": {
		"resolved":       "Resolved — the issue is settled",
		"appeal_upheld":  "Appeal upheld — the suspension was contested and reversed",
		"operator_error": "Operator error — suspended in error",
		"voluntary_end":  "Voluntary end — the merchant asked to resume",
	},
	"trial_extend": {
		"support_escalation": "Support escalation — an open case needs more time",
		"onboarding_delay":   "Onboarding delay — setup slipped, outside the merchant's control",
		"billing_dispute":    "Billing dispute — open; the trial should not lapse meanwhile",
		"goodwill":           "Goodwill — discretionary grant, no other category applies",
		"operator_error":     "Operator error — correcting a mistaken earlier extension",
	},
	"purge": {
		"merchant_request": "Merchant request — the merchant asked for deletion",
		"erasure_request":  "Erasure request — a statutory demand (GDPR art.17)",
		"fraud":            "Fraud — confirmed fraudulent tenant, removed after investigation",
		"abandoned":        "Abandoned — onboarding never completed",
		"legal":            "Legal — a demand other than erasure",
		"operator_error":   "Operator error — created in error, or a test tenant",
	},
}

// lifecycleVerbCodes maps each verb to its authoritative code list. Ordered
// deliberately: suspend and unsuspend first because §8.8 requires them of
// every implementer, the two mark8ly-specific verbs after.
var lifecycleVerbCodes = []struct {
	Verb  string
	Codes []string
}{
	{"suspend", SuspendReasonCodes},
	{"unsuspend", UnsuspendReasonCodes},
	{"trial_extend", ExtendReasonCodes},
	{"purge", PurgeReasonCodes},
}

// labelFor returns the wording for one code, falling back to the code itself.
//
// The fallback is never expected to fire — the coverage test makes a missing
// label a build failure — but it must not be an empty string. §8.8 requires a
// non-empty label on every entry, so an empty one would take a conforming
// endpoint out of conformance to report a labelling mistake. The code is an
// ugly menu option and an honest one.
func labelFor(verb, code string) string {
	if label, ok := reasonCodeLabels[verb][code]; ok && label != "" {
		return label
	}
	return code
}

// LifecycleReasonCodes builds the §8.8 payload from the authoritative lists.
//
// Exported so the console-facing shape can be asserted from a test without
// standing up a router, and so nothing is tempted to rebuild it by hand.
func LifecycleReasonCodes() map[string][]ReasonCode {
	out := make(map[string][]ReasonCode, len(lifecycleVerbCodes))
	for _, verb := range lifecycleVerbCodes {
		codes := make([]ReasonCode, 0, len(verb.Codes))
		for _, code := range verb.Codes {
			codes = append(codes, ReasonCode{Code: code, Label: labelFor(verb.Verb, code)})
		}
		out[verb.Verb] = codes
	}
	return out
}

// LifecycleReasonCodesHandler serves contract §8.8.
//
// It holds nothing. The response is a constant folded out of this package's
// own `var` blocks, which is exactly the point of the endpoint: §8.3 made the
// codes REQUIRED on suspend and unsuspend and said nothing about how a caller
// was meant to learn them, so the console hand-copied mark8ly's out of
// `tenant_lifecycle.go` (tesserix-home#345). A copied vocabulary drifts in the
// silent direction — a code added here is simply missing from the operator's
// menu, and they pick the nearest wrong one.
type LifecycleReasonCodesHandler struct{}

func NewLifecycleReasonCodesHandler() *LifecycleReasonCodesHandler {
	return &LifecycleReasonCodesHandler{}
}

// Register mounts the route unconditionally, unlike the write endpoints it
// describes.
//
// Those refuse to mount without an audit path, because an unattributed write
// is worse than an absent one. This is a read of a compile-time constant: it
// depends on nothing, can fail in no way, and gating it on the writes being
// wired would answer 404 in exactly the deployment where an operator most
// needs to see what the vocabulary is.
func (h *LifecycleReasonCodesHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/lifecycle/reason-codes", h.reasonCodes)
}

func (h *LifecycleReasonCodesHandler) reasonCodes(c *gin.Context) {
	c.JSON(http.StatusOK, LifecycleReasonCodesResponse{Data: LifecycleReasonCodes()})
}
