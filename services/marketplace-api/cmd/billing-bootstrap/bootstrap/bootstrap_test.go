package bootstrap_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
