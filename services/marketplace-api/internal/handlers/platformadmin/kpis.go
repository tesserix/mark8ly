package platformadmin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/estatecounts"
	"github.com/mark8ly/marketplace-api/internal/onboardingfunnel"
)

// EstateCounts is the subset of estatecounts.Client this handler needs.
// Declared here (rather than importing the concrete client type directly
// into Deps) so the handler can be tested with a stub, mirroring
// OnboardingFunnel in onboarding.go.
type EstateCounts interface {
	Get(ctx context.Context) (*estatecounts.Counts, error)
}

// Subscriptions is the subset of subscription.Repository this handler
// needs. Narrowed to one method for the same reason as EstateCounts above.
type Subscriptions interface {
	CountTrialsExpiring(ctx context.Context, db *gorm.DB, asOf time.Time) (int64, error)
}

// kpiKey is one metric the console may ask for.
type kpiKey struct {
	Name         string
	Instrumented bool
}

// KPIRegistry declares EVERY key mark8ly knows. Both the 200 payload and
// the 501 responses are driven from here rather than from conditionals in
// the handler, so a key cannot be silently dropped.
//
// An uninstrumented key is NOT omitted and NOT zero — the console has
// rendered em-dashes that look like zeroes for a year because a sibling
// route falls through to {}. See docs/ops for the incident writeup.
var KPIRegistry = []kpiKey{
	{Name: "tenants_active", Instrumented: true},
	{Name: "stores_active", Instrumented: true},
	{Name: "onboarding_in_flight", Instrumented: true},
	{Name: "trials_expiring", Instrumented: true},
	// GMV is genuinely not computable: currency is per store and per order,
	// and there is no FX source anywhere in the workspace. 501 is the
	// honest answer; 0 would be a lie and omitting it would be ambiguous.
	{Name: "gmv_today", Instrumented: false},
	{Name: "gmv_month", Instrumented: false},
}

func lookupKPI(name string) (kpiKey, bool) {
	for _, k := range KPIRegistry {
		if k.Name == name {
			return k, true
		}
	}
	return kpiKey{}, false
}

func defaultKPIKeys() []string {
	keys := make([]string, 0, len(KPIRegistry))
	for _, k := range KPIRegistry {
		if k.Instrumented {
			keys = append(keys, k.Name)
		}
	}
	return keys
}

// parseKPIKeys parses ?keys= as a comma-separated list, trimming each.
// Empty or absent means "all instrumented keys".
func parseKPIKeys(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultKPIKeys()
	}
	parts := strings.Split(raw, ",")
	keys := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			keys = append(keys, p)
		}
	}
	if len(keys) == 0 {
		return defaultKPIKeys()
	}
	return keys
}

// KPIsHandler serves the platform console's GET /admin/kpis (#282). It
// gathers values from three independent upstreams — estatecounts,
// onboardingfunnel, and the subscription repository — and only ever
// answers with the complete set or a 503; see kpis() for why.
type KPIsHandler struct {
	estate EstateCounts
	funnel OnboardingFunnel
	subs   Subscriptions
	db     *gorm.DB
	logger *slog.Logger
	// now is overridable in tests; production uses time.Now.
	now func() time.Time
}

// NewKPIsHandler constructs the handler. logger may be nil.
func NewKPIsHandler(estate EstateCounts, funnel OnboardingFunnel, subs Subscriptions, db *gorm.DB, logger *slog.Logger) *KPIsHandler {
	return &KPIsHandler{estate: estate, funnel: funnel, subs: subs, db: db, logger: logger, now: time.Now}
}

// Register mounts the route on the supplied group.
func (h *KPIsHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/kpis", h.kpis)
}

func (h *KPIsHandler) kpis(c *gin.Context) {
	keys := parseKPIKeys(c.Query("keys"))

	// Resolve every requested key against the registry BEFORE touching any
	// upstream. The two 501 cases share a status but differ in message —
	// see the spec — and either one aborts the whole request; there is no
	// partial success to accumulate first.
	for _, name := range keys {
		key, known := lookupKPI(name)
		switch {
		case !known:
			c.JSON(http.StatusNotImplemented, gin.H{
				"error":   "not_implemented",
				"message": fmt.Sprintf("kpi %q is not a recognised key", name),
				"key":     name,
			})
			return
		case !key.Instrumented:
			c.JSON(http.StatusNotImplemented, gin.H{
				"error":   "not_implemented",
				"message": fmt.Sprintf("kpi %q is known but not instrumented", name),
				"key":     name,
			})
			return
		}
	}

	ctx := c.Request.Context()

	counts, err := h.estate.Get(ctx)
	if err != nil {
		h.respondUnavailable(c, "estatecounts", err)
		return
	}

	funnelStats, err := h.funnel.GetFunnel(ctx, onboardingfunnel.FunnelParams{})
	if err != nil {
		h.respondUnavailable(c, "onboardingfunnel", err)
		return
	}

	trialsExpiring, err := h.subs.CountTrialsExpiring(ctx, h.db, h.now())
	if err != nil {
		h.respondUnavailable(c, "subscription", err)
		return
	}

	values := map[string]int64{
		"tenants_active":       counts.TenantsActive,
		"stores_active":        counts.StoresActive,
		"onboarding_in_flight": funnelStats.InFlight,
		"trials_expiring":      trialsExpiring,
	}

	data := gin.H{}
	for _, name := range keys {
		data[name] = values[name]
	}

	c.JSON(http.StatusOK, gin.H{"data": data})
}

// respondUnavailable answers 503 upstream_unavailable. This is the ONLY
// error path for the gather stage: an estatecounts/onboardingfunnel
// ErrUnavailable and a bare subscription-repository error are all treated
// identically, because a partial KPI object is indistinguishable from a
// complete one — the very failure mode this endpoint exists to close.
func (h *KPIsHandler) respondUnavailable(c *gin.Context, source string, err error) {
	if h.logger != nil {
		h.logger.Error("kpis upstream unavailable", "source", source, "err", err)
	}
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"error": "upstream_unavailable", "message": "one or more kpi sources is unavailable",
	})
}
