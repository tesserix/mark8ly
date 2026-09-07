// services/marketplace-api/internal/handlers/platformadmin/billing_entitlements.go
package platformadmin

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/plangate"
)

// entitlementsSource names the product this matrix belongs to. It matches the
// `source` vocabulary the console's plan_catalog_entitlements table CHECKs
// (tesserix-home#146), which is closed at 'mark8ly' today: the column is the
// seam for a second product, not a live multi-source surface.
const entitlementsSource = "mark8ly"

// EntitlementsResponse is the wire shape of GET /admin/billing/entitlements.
//
// Deliberately NOT wrapped in a `data` envelope, matching
// LifecycleReasonCodesResponse's sibling reads on this surface only in
// spirit: the four fields are all top-level because a consumer parity-checking
// this payload compares whole documents, and an envelope adds a level with
// nothing in it.
//
// Plans is map[plan]map[feature]int rather than a list of rows so the shape
// mirrors plangate's own two-lookup model exactly — one integer per
// (plan, feature), read as an entitlement by IsAllowed and as a limit by
// Limit. Splitting it into "enabled" and "limit" here would invent a
// distinction the enforcement point does not make, and give the console two
// things to keep agreeing instead of one.
type EntitlementsResponse struct {
	// Source is the product this matrix is enforced by.
	Source string `json:"source"`

	// CatalogMode is CONSOLE_CATALOG_MODE as this process actually read it —
	// `test` today, moving at the Stripe live-key swap (mark8ly#371). The
	// console cannot see that variable and the value is not derivable from
	// anything it can see, which is the whole reason it is reported here
	// rather than assumed on the other side. It is passed through verbatim,
	// including empty: an unset mode must read as unset, never as `test`.
	CatalogMode string `json:"catalog_mode"`

	// Features is plangate's canonical ordered feature list. Order is part of
	// the contract, not incidental — a consumer diffing this positionally must
	// see churn only when the gate changes.
	Features []string `json:"features"`

	// Plans carries every plan the matrix keys on. A plan absent from the
	// matrix is absent here rather than published as all-Disabled; see
	// plangate.AllPlans for why that distinction matters for
	// subscription.PlanMarketplace.
	Plans map[string]map[string]int `json:"plans"`
}

// BillingEntitlementsHandler serves GET /admin/billing/entitlements — the
// compiled plan-feature matrix, exactly as this binary enforces it.
//
// The endpoint exists so the platform console can parity-check the
// entitlements it stores and displays against what is actually enforced
// (tesserix-home#146). That purpose is the constraint on the implementation:
// **every value and every key in the response is DERIVED from plangate at
// request time and none is restated here.** A hand-typed feature name, plan
// name or limit would make this file a third copy of the matrix, free to
// disagree with the gate it claims to describe — and a parity check between
// two copies of the same mistake reports agreement, which is worse than no
// check at all.
//
// It holds no repository and touches no database. The matrix is compiled into
// the binary, so this read cannot fail and needs nothing wired.
type BillingEntitlementsHandler struct {
	catalogMode string
	logger      *slog.Logger
}

// NewBillingEntitlementsHandler builds the handler. catalogMode comes from the
// same configuration value CONSOLE_CATALOG_MODE feeds (cfg.ConsoleCatalogMode),
// injected rather than read from the environment here so the response describes
// the mode THIS process runs with and a test can state one without setting a
// variable.
func NewBillingEntitlementsHandler(catalogMode string, logger *slog.Logger) *BillingEntitlementsHandler {
	return &BillingEntitlementsHandler{catalogMode: catalogMode, logger: logger}
}

// Register mounts the route unconditionally, for the same reason
// LifecycleReasonCodesHandler does: it reads a compile-time constant, depends
// on nothing and can fail in no way. Gating it on any of this surface's
// optional dependencies would answer 404 in exactly the deployment where the
// console most needs to see what is enforced.
//
// It needs no packages/platformauth entry. RequiredWriteCapabilities covers
// writes only, and RequiredReadCapabilities is opt-in — every read this
// surface already serves in production (/admin/audit-logs,
// /admin/billing/subscriptions, /admin/billing/trials, /admin/health) is
// absent from it and must stay absent. This is a read of the same kind, so it
// declares nothing and behaves exactly as its siblings do: signature only.
func (h *BillingEntitlementsHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/billing/entitlements", h.list)
}

func (h *BillingEntitlementsHandler) list(c *gin.Context) {
	features := plangate.AllFeatures()
	names := make([]string, 0, len(features))
	for _, f := range features {
		names = append(names, string(f))
	}

	plans := make(map[string]map[string]int, len(plangate.AllPlans()))
	for _, p := range plangate.AllPlans() {
		// AllFeatureLimits is the canonical accessor: it returns EVERY
		// feature for the plan, defaulting an unset cell to Disabled (0), so
		// the response can never carry a hole a consumer might read as
		// permissive.
		plans[string(p)] = plangate.AllFeatureLimits(p)
	}

	c.JSON(http.StatusOK, EntitlementsResponse{
		Source:      entitlementsSource,
		CatalogMode: h.catalogMode,
		Features:    names,
		Plans:       plans,
	})
}
