package bootstrap_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/cmd/billing-bootstrap/bootstrap"
	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
	stripec "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

// stripeMock is an in-memory Stripe API stub sufficient for bootstrap testing.
// It stores Products and Prices and records how many creates occurred.
type stripeMock struct {
	mu sync.Mutex
	// products: planKey -> full product JSON map (including nested metadata)
	products map[string]map[string]any
	// pricesByLookup: lookupKey -> price JSON map
	pricesByLookup map[string]map[string]any

	productCreates int
	priceCreates   int
}

func newStripeMock() *stripeMock {
	return &stripeMock{
		products:       make(map[string]map[string]any),
		pricesByLookup: make(map[string]map[string]any),
	}
}

func (m *stripeMock) resetCounters() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.productCreates = 0
	m.priceCreates = 0
}

func (m *stripeMock) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Request-Id", "req_test")
		m.mu.Lock()
		defer m.mu.Unlock()

		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/products"):
			// Return all stored products with metadata so FindProductByMetadata can match.
			var list []map[string]any
			for _, p := range m.products {
				list = append(list, p)
			}
			if list == nil {
				list = []map[string]any{}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": list})

		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/products"):
			b, _ := io.ReadAll(r.Body)
			v, _ := url.ParseQuery(string(b))
			plan := v.Get("metadata[plan]")
			id := "prod_" + plan
			product := map[string]any{
				"id":       id,
				"name":     v.Get("name"),
				"active":   true,
				"metadata": map[string]string{"plan": plan},
			}
			m.products[plan] = product
			m.productCreates++
			_ = json.NewEncoder(w).Encode(product)

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/prices"):
			q := r.URL.Query()
			lookup := ""
			// stripe-go SDK v82 serializes []*string slices with indexed brackets
			// (lookup_keys[0]=), unlike the previous raw-HTTP form (lookup_keys[]=).
			if lookups, ok := q["lookup_keys[0]"]; ok && len(lookups) > 0 {
				lookup = lookups[0]
			} else if lookups, ok := q["lookup_keys[]"]; ok && len(lookups) > 0 {
				lookup = lookups[0]
			}
			var list []map[string]any
			if lookup != "" {
				if p, ok := m.pricesByLookup[lookup]; ok {
					list = append(list, p)
				}
			}
			if list == nil {
				list = []map[string]any{}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": list})

		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/prices"):
			b, _ := io.ReadAll(r.Body)
			v, _ := url.ParseQuery(string(b))
			lookup := v.Get("lookup_key")
			id := "price_" + lookup
			price := map[string]any{
				"id":         id,
				"lookup_key": lookup,
				"currency":   v.Get("currency"),
				"active":     true,
			}
			if ua := v.Get("unit_amount"); ua != "" {
				n, _ := strconv.ParseInt(ua, 10, 64)
				price["unit_amount"] = n
			}
			if tb := v.Get("tax_behavior"); tb != "" {
				price["tax_behavior"] = tb
			}
			m.pricesByLookup[lookup] = price
			m.priceCreates++
			_ = json.NewEncoder(w).Encode(price)

		default:
			http.NotFound(w, r)
		}
	})
}

func TestBootstrap_FirstRunCreatesEverything(t *testing.T) {
	m := newStripeMock()
	srv := httptest.NewServer(m.handler())
	defer srv.Close()

	c := stripec.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	descriptors := pricing.AllDescriptors()
	err := bootstrap.Run(context.Background(), c, descriptors, nil)
	require.NoError(t, err)

	// 3 unique plans: starter, studio, pro.
	require.Equal(t, 3, m.productCreates, "should create 3 products (starter/studio/pro)")
	// One price per descriptor.
	require.Equal(t, len(descriptors), m.priceCreates, "should create one price per descriptor")
}

func TestBootstrap_SecondRunIsNoop(t *testing.T) {
	m := newStripeMock()
	srv := httptest.NewServer(m.handler())
	defer srv.Close()

	c := stripec.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	descriptors := pricing.AllDescriptors()
	require.NoError(t, bootstrap.Run(context.Background(), c, descriptors, nil))

	m.resetCounters()
	require.NoError(t, bootstrap.Run(context.Background(), c, descriptors, nil))

	require.Zero(t, m.productCreates, "second run must not create products")
	require.Zero(t, m.priceCreates, "second run must not create prices")
}

func TestBootstrap_AbortsOnErrorEarly(t *testing.T) {
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"type":"api_error","code":"server_unreachable"}}`))
	}))
	defer failSrv.Close()

	c := stripec.New("sk_test_x")
	c.SetBaseURLForTesting(failSrv.URL)

	err := bootstrap.Run(context.Background(), c, pricing.AllDescriptors(), nil)
	require.Error(t, err)
}

// #459: bootstrap was create-only. An existing lookup_key meant "reuse",
// full stop — the amount was never compared. So a price change whose key
// does not change was SILENTLY a no-op: bootstrap logged "reusing price"
// and exited 0, Stripe kept charging the old amount, and the console
// showed the new one. The operator saw a successful publish.
//
// The parity check cannot catch this either: it compares the console's
// catalog to Stripe, so it is structurally blind to a divergence the
// console does not know about.
//
// Resolved 2026-08-29: lookup_key is STABLE IDENTITY, and bootstrap
// REFUSES on a differing amount rather than updating. Stripe Prices are
// immutable in amount, so "change the price" really means create-new-and-
// migrate-subscribers — something mark8ly has never done and has no
// runbook for. Refusing turns a silent no-op into a loud stop.
func TestBootstrap_RefusesWhenExistingPriceAmountDiffers(t *testing.T) {
	m := newStripeMock()
	srv := httptest.NewServer(m.handler())
	defer srv.Close()
	c := stripec.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	descriptors := []pricing.PriceDescriptor{oneDescriptor(t)}

	// First run creates the price at the catalog's amount.
	require.NoError(t, bootstrap.Run(context.Background(), c, descriptors, nil))
	require.Equal(t, 1, m.priceCreates)

	// Stripe now holds a DIFFERENT amount under the same lookup_key —
	// exactly the state a changed catalog amount produces.
	m.mu.Lock()
	m.pricesByLookup[descriptors[0].LookupKey]["unit_amount"] = int64(999999)
	m.mu.Unlock()
	m.resetCounters()

	err := bootstrap.Run(context.Background(), c, descriptors, nil)

	require.Error(t, err, "a differing amount must not be silently reused")
	require.Contains(t, err.Error(), descriptors[0].LookupKey,
		"the error must name the key so an operator knows which price to look at")
	require.Contains(t, err.Error(), "999999", "the error must show what Stripe holds")
	require.Equal(t, 0, m.priceCreates,
		"refusing means creating nothing — not quietly creating a second price")
}

// The other half: an unchanged amount must still be a clean no-op, or
// every re-run of a correct bootstrap would now fail.
func TestBootstrap_ReusesWhenAmountMatches(t *testing.T) {
	m := newStripeMock()
	srv := httptest.NewServer(m.handler())
	defer srv.Close()
	c := stripec.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	descriptors := []pricing.PriceDescriptor{oneDescriptor(t)}

	require.NoError(t, bootstrap.Run(context.Background(), c, descriptors, nil))
	m.resetCounters()

	require.NoError(t, bootstrap.Run(context.Background(), c, descriptors, nil),
		"an unchanged catalog must remain idempotent")
	require.Equal(t, 0, m.priceCreates)
}

// oneDescriptor returns a single real catalog descriptor, so the test runs
// against the shape bootstrap actually receives rather than a fabricated one.
func oneDescriptor(t *testing.T) pricing.PriceDescriptor {
	t.Helper()
	all := pricing.AllDescriptors()
	require.NotEmpty(t, all)
	return all[0]
}
