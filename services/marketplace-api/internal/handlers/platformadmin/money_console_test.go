package platformadmin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/consolecatalog"
	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
)

// resolveMoney prices from the compiled catalog, which is exactly what an
// unconfigured console gives this surface (see compiledPriceCatalog: the nil
// CatalogResolver is the cutover's kill switch).
//
// It exists so money_test.go — every rule this helper has to honour, written
// before the console was involved at all — keeps testing the ROLLBACK path
// unchanged, byte for byte, after the cutover. If those assertions ever stop
// holding, the config change that reverts tesserix-home#328 phase C no
// longer restores today's behaviour, which is the thing the kill switch is
// for.
func resolveMoney(plan, period string, billingCurrency *string, tier subscription.PriceTier) (money, bool) {
	return compiledPriceCatalog().resolveMoney(plan, period, billingCurrency, tier)
}

// stubResolver is a CatalogResolver returning a fixed Resolution, recording
// the context it was handed.
type stubResolver struct {
	res     consolecatalog.Resolution
	gotCtx  context.Context
	callCnt int
}

func (s *stubResolver) Resolve(ctx context.Context) consolecatalog.Resolution {
	s.gotCtx = ctx
	s.callCnt++
	return s.res
}

func freshResolution(prices ...consolecatalog.Price) consolecatalog.Resolution {
	return consolecatalog.Resolution{
		Catalog: consolecatalog.Catalog{
			Mode:       "test",
			RevisionID: consolecatalog.SharedRevisionID,
			Prices:     prices,
		},
		Source:    consolecatalog.SourceFresh,
		FetchedAt: time.Now(),
	}
}

// consoleRowsFromCompiled flattens the compiled catalog into the row shape
// the console publishes.
//
// Deliberately written out here rather than calling
// consolecatalog.CompiledCatalog: that function is the FALLBACK this cutover
// falls back TO, so using it to build the "console" side of a comparison
// would compare a thing to itself. This walks pricing.AllDescriptors by a
// separate route, so a test that says "the console and the compiled catalog
// agree" is comparing two independently produced row sets.
func consoleRowsFromCompiled(t *testing.T) []consolecatalog.Price {
	t.Helper()
	var out []consolecatalog.Price
	for _, d := range pricing.AllDescriptors() {
		for cur, amt := range d.Options {
			// A zero-value Amount is a catalog GAP, not a price of zero —
			// pricing's Options map is pre-populated for every developed
			// currency. The console has no such row to publish.
			if amt.Currency == "" {
				continue
			}
			out = append(out, consolecatalog.Price{
				LookupKey:       d.LookupKey,
				Plan:            string(d.Plan),
				Period:          string(d.Period),
				Tier:            string(d.Tier),
				Currency:        cur,
				UnitAmountMinor: amt.UnitAmountMinor,
				TaxBehavior:     amt.TaxBehavior,
			})
		}
	}
	require.NotEmpty(t, out)
	return out
}

// moneyCase is one (plan, period, currency, tier) question, covering exactly
// the combinations money_test.go asserts on plus the two unpriced plans.
type moneyCase struct {
	plan, period, currency string
	tier                   subscription.PriceTier
}

func moneyCases() []moneyCase {
	return []moneyCase{
		{"starter", "monthly", "gbp", subscription.PriceTierDeveloped},
		{"starter", "monthly", "usd", subscription.PriceTierDeveloped},
		{"starter", "monthly", "inr", subscription.PriceTierPPP},
		{"starter", "annual", "gbp", subscription.PriceTierDeveloped},
		{"studio", "monthly", "eur", subscription.PriceTierDeveloped},
		{"pro", "annual", "vnd", subscription.PriceTierPPP},
		{"trial", "monthly", "usd", subscription.PriceTierDeveloped},
		{"marketplace", "monthly", "usd", subscription.PriceTierDeveloped},
		{"nonexistent", "monthly", "usd", subscription.PriceTierDeveloped},
		{"starter", "monthly", "zzz", subscription.PriceTierDeveloped},
		// inr has no developed-tier price: asking for one must miss rather
		// than fall through to the PPP row that does exist for it.
		{"starter", "monthly", "inr", subscription.PriceTierDeveloped},
	}
}

// TestConsoleCatalogAgreesWithCompiled is the cutover's core claim: for
// every question this surface asks, a console catalog and the compiled
// catalog return the same answer — including the same ok=false.
//
// The parity monitor already reports differences=0, but it compares
// (lookup_key, currency) AMOUNTS. It says nothing about whether this
// package's plan/period/tier/currency lookup finds the same row, which is
// the part the cutover actually depends on.
func TestConsoleCatalogAgreesWithCompiled(t *testing.T) {
	console := priceCatalog{prices: consolecatalog.Catalog{
		Prices: consoleRowsFromCompiled(t),
	}.Index()}
	compiled := compiledPriceCatalog()

	for _, tc := range moneyCases() {
		cur := tc.currency
		wantMoney, wantOK := compiled.resolveMoney(tc.plan, tc.period, &cur, tc.tier)
		gotMoney, gotOK := console.resolveMoney(tc.plan, tc.period, &cur, tc.tier)

		require.Equal(t, wantOK, gotOK, "ok differs for %+v", tc)
		require.Equal(t, wantMoney, gotMoney, "money differs for %+v", tc)
	}
}

// TestConsoleCatalogResolvesKnownAmounts pins the amounts money_test.go
// pins, resolved through the CONSOLE path rather than the compiled one, so a
// change that silently made the console path return nothing at all could not
// pass by agreeing with an equally broken compiled path.
func TestConsoleCatalogResolvesKnownAmounts(t *testing.T) {
	console := priceCatalog{prices: consolecatalog.Catalog{
		Prices: consoleRowsFromCompiled(t),
	}.Index()}

	gbp := "gbp"
	m, ok := console.resolveMoney("starter", "monthly", &gbp, subscription.PriceTierDeveloped)
	require.True(t, ok)
	require.Equal(t, money{Amount: 1500, Currency: "GBP"}, m)

	inr := "inr"
	m, ok = console.resolveMoney("starter", "monthly", &inr, subscription.PriceTierPPP)
	require.True(t, ok)
	require.Equal(t, money{Amount: 99900, Currency: "INR"}, m)
}

// TestConsoleCatalogMissingEntryIsNotZero is the property the previous
// implementation held with an `amt.Currency != ""` guard: a combination the
// catalog has no row for resolves as ok=false, never as amount=0. A console
// row set is built from real rows, so the zero-value-placeholder mechanism
// does not exist there — but "absent resolves to zero" must be impossible
// regardless of which source is answering.
func TestConsoleCatalogMissingEntryIsNotZero(t *testing.T) {
	// One plan/period/tier, one currency. Every other question is a miss.
	console := priceCatalog{prices: consolecatalog.Catalog{Prices: []consolecatalog.Price{
		{LookupKey: "k", Plan: "starter", Period: "monthly", Tier: "developed",
			Currency: "gbp", UnitAmountMinor: 1500},
	}}.Index()}

	for _, tc := range []moneyCase{
		{"starter", "monthly", "usd", subscription.PriceTierDeveloped}, // currency absent
		{"starter", "annual", "gbp", subscription.PriceTierDeveloped},  // period absent
		{"studio", "monthly", "gbp", subscription.PriceTierDeveloped},  // plan absent
		{"starter", "monthly", "gbp", subscription.PriceTierPPP},       // tier absent
	} {
		cur := tc.currency
		m, ok := console.resolveMoney(tc.plan, tc.period, &cur, tc.tier)
		require.False(t, ok, "%+v must miss", tc)
		require.Equal(t, money{}, m, "%+v must not resolve to an amount at all", tc)
		require.Zero(t, m.Amount, "%+v must never surface as amount=0", tc)
	}
}

// TestConsoleCatalogCaseHandling walks the three cases the wire contract
// spans in one go: the catalog key is lowercase, the DB's char(3) column is
// of unspecified case, and the response must be uppercase.
func TestConsoleCatalogCaseHandling(t *testing.T) {
	console := priceCatalog{prices: consolecatalog.Catalog{Prices: []consolecatalog.Price{
		// Lowercase, as plan_catalog_amounts' currency CHECK constrains it.
		{LookupKey: "k", Plan: "starter", Period: "monthly", Tier: "developed",
			Currency: "gbp", UnitAmountMinor: 1500},
	}}.Index()}

	for _, dbValue := range []string{"gbp", "GBP", "GbP", " gbp "} {
		v := dbValue
		m, ok := console.resolveMoney("starter", "monthly", &v, subscription.PriceTierDeveloped)
		require.True(t, ok, "billing_currency %q must resolve", dbValue)
		require.Equal(t, "GBP", m.Currency, "the wire contract is uppercase")
		require.Equal(t, int64(1500), m.Amount)
	}

	// A console that ever published an uppercase currency must still be
	// found: the fold is applied to both sides of the comparison, not only
	// to the caller's value.
	upperConsole := priceCatalog{prices: consolecatalog.Catalog{Prices: []consolecatalog.Price{
		{LookupKey: "k", Plan: "Starter", Period: "Monthly", Tier: "Developed",
			Currency: "GBP", UnitAmountMinor: 1500},
	}}.Index()}
	cur := "gbp"
	m, ok := upperConsole.resolveMoney("starter", "monthly", &cur, subscription.PriceTierDeveloped)
	require.True(t, ok)
	require.Equal(t, money{Amount: 1500, Currency: "GBP"}, m)
}

// TestResolvePriceCatalogNilResolverIsCompiled pins the kill switch: no
// resolver at all must price exactly as the compiled catalog does, with no
// panic and no empty answer.
func TestResolvePriceCatalogNilResolverIsCompiled(t *testing.T) {
	var pc priceCatalog
	require.NotPanics(t, func() {
		pc = resolvePriceCatalog(context.Background(), nil, nil, nil)
	})

	gbp := "gbp"
	m, ok := pc.resolveMoney("starter", "monthly", &gbp, subscription.PriceTierDeveloped)
	require.True(t, ok, "the rollback path must still be able to price")
	require.Equal(t, money{Amount: 1500, Currency: "GBP"}, m)
}

// TestResolvePriceCatalogFailedResolverStillPrices covers the cache's own
// fallback reaching this package: Resolve never errors, it answers with the
// compiled catalog and Source=compiled. This surface must price from it
// normally.
func TestResolvePriceCatalogFailedResolverStillPrices(t *testing.T) {
	r := &stubResolver{res: consolecatalog.Resolution{
		Catalog: consolecatalog.CompiledCatalog("test"),
		Source:  consolecatalog.SourceCompiled,
		Stale:   true,
		Err:     errors.New("console unreachable"),
	}}

	pc := resolvePriceCatalog(context.Background(), r, newCatalogFallbackLog(), slog.Default())

	gbp := "gbp"
	m, ok := pc.resolveMoney("starter", "monthly", &gbp, subscription.PriceTierDeveloped)
	require.True(t, ok)
	require.Equal(t, money{Amount: 1500, Currency: "GBP"}, m)
}

// TestResolvePriceCatalogEmptyCatalogFallsBackToCompiled: a resolution that
// carries no usable prices would strip the amount from every row on the
// page, which reads as "these merchants have no price" rather than as a
// fault. Fall back rather than serve that.
func TestResolvePriceCatalogEmptyCatalogFallsBackToCompiled(t *testing.T) {
	r := &stubResolver{res: freshResolution()}

	pc := resolvePriceCatalog(context.Background(), r, newCatalogFallbackLog(), slog.Default())

	gbp := "gbp"
	m, ok := pc.resolveMoney("starter", "monthly", &gbp, subscription.PriceTierDeveloped)
	require.True(t, ok, "an empty resolution must not leave the surface unable to price")
	require.Equal(t, money{Amount: 1500, Currency: "GBP"}, m)
}

// TestResolvePriceCatalogPassesCallerContext proves the caller's context
// reaches Resolve — the console read has to be cancellable by the request
// that triggered it.
func TestResolvePriceCatalogPassesCallerContext(t *testing.T) {
	r := &stubResolver{res: freshResolution(consolecatalog.Price{
		LookupKey: "k", Plan: "starter", Period: "monthly", Tier: "developed",
		Currency: "gbp", UnitAmountMinor: 1500,
	})}

	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "caller")

	resolvePriceCatalog(ctx, r, newCatalogFallbackLog(), nil)

	require.Equal(t, 1, r.callCnt)
	require.Equal(t, "caller", r.gotCtx.Value(ctxKey{}), "Resolve must get the caller's context")
}

// stubTrialLister returns a fixed page of trials.
type stubTrialLister struct{ rows []trial.ExpiringRow }

func (s stubTrialLister) ListExpiring(_ context.Context, _ *gorm.DB, _ time.Time, _ time.Duration, _, _ int, _ trial.ListOptions) ([]trial.ExpiringRow, int64, error) {
	return s.rows, int64(len(s.rows)), nil
}

// TestBillingTrialsUsesRequestContextAndResolvesOncePerPage pins the two
// wiring properties the row-level helpers cannot: the handler hands Resolve
// the REQUEST's context (not context.Background(), which would leave a
// console read running after the client hung up), and it resolves ONCE for
// the page rather than once per row — a page priced from two different
// catalogs is a page that can contradict itself.
func TestBillingTrialsUsesRequestContextAndResolvesOncePerPage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	usd := "usd"
	rows := make([]trial.ExpiringRow, 0, 3)
	for i := 0; i < 3; i++ {
		rows = append(rows, trial.ExpiringRow{
			TenantID: "t", StoreID: "s", TrialEndsAt: time.Now().Add(48 * time.Hour),
			Plan: "starter", Period: "monthly", BillingCurrency: &usd,
			PriceTier: subscription.PriceTierDeveloped, Status: "trialing",
		})
	}

	r := &stubResolver{res: freshResolution(consolecatalog.Price{
		LookupKey: "k", Plan: "starter", Period: "monthly", Tier: "developed",
		Currency: "usd", UnitAmountMinor: 4242,
	})}

	// The page has rows, so lookupTenantNames DOES call the directory; a nil
	// one would panic. The stub returns no tenants, which only means the
	// rows carry no tenant_name — irrelevant to what this test asserts.
	h := NewBillingTrialsHandler(stubTrialLister{rows: rows}, stubDirectory{}, nil, time.Now, r, nil)

	engine := gin.New()
	h.Register(engine.Group(""))

	type ctxKey struct{}
	req := httptest.NewRequest(http.MethodGet, "/admin/billing/trials", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxKey{}, "request"))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, 1, r.callCnt, "one page must resolve the catalog exactly once")
	require.Equal(t, "request", r.gotCtx.Value(ctxKey{}),
		"the console read must run on the request's context, never context.Background()")
	require.Contains(t, w.Body.String(), `"amount":4242`,
		"the page must be priced from the console catalog, not the compiled one")
}

// TestCatalogFallbackLogThrottles pins the observability decision: the first
// degradation is reported immediately, the storm behind it is counted rather
// than printed, and a fresh resolution is never reported at all.
func TestCatalogFallbackLogThrottles(t *testing.T) {
	var lines []string
	log := slog.New(slog.NewTextHandler(&lineWriter{&lines}, nil))

	now := time.Now()
	fb := newCatalogFallbackLog()
	fb.now = func() time.Time { return now }

	stale := consolecatalog.Resolution{
		Catalog: consolecatalog.Catalog{RevisionID: "r"},
		Source:  consolecatalog.SourceStale, Stale: true,
		Err: errors.New("console unreachable"),
	}

	fb.note(log, stale)
	require.Len(t, lines, 1, "the first degradation must be reported immediately")

	for i := 0; i < 20; i++ {
		fb.note(log, stale)
	}
	require.Len(t, lines, 1, "a refresh storm must not produce a line per request")

	now = now.Add(catalogFallbackLogInterval + time.Second)
	fb.note(log, stale)
	require.Len(t, lines, 2)
	require.Contains(t, lines[1], "suppressed_since_last_line=20",
		"the second line must account for what it stands for")

	fb.note(log, freshResolution())
	require.Len(t, lines, 2, "a fresh resolution is not a degradation and must be silent")
}

// TestCatalogFallbackLogNilSafe: observability must not be able to take down
// the surface it observes.
func TestCatalogFallbackLogNilSafe(t *testing.T) {
	var nilLog *catalogFallbackLog
	require.NotPanics(t, func() {
		nilLog.note(slog.Default(), consolecatalog.Resolution{Source: consolecatalog.SourceStale})
		newCatalogFallbackLog().note(nil, consolecatalog.Resolution{Source: consolecatalog.SourceStale})
	})
}

// stubDirectory answers the tenant-name lookup with nothing.
type stubDirectory struct{}

func (stubDirectory) List(context.Context, tenantdirectory.ListParams) (*tenantdirectory.ListResult, error) {
	return &tenantdirectory.ListResult{}, nil
}

func (stubDirectory) Get(context.Context, string) (*tenantdirectory.TenantDetail, error) {
	return nil, errors.New("not used")
}

func (stubDirectory) FindByOwnerEmail(context.Context, string) (*tenantdirectory.Tenant, error) {
	return nil, errors.New("not used")
}

type lineWriter struct{ lines *[]string }

func (w *lineWriter) Write(p []byte) (int, error) {
	*w.lines = append(*w.lines, strings.TrimRight(string(p), "\n"))
	return len(p), nil
}
