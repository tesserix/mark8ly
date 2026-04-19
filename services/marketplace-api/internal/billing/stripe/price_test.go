package stripe_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

func TestCreatePrice_EmitsCurrencyOptionsWithAUExclusive(t *testing.T) {
	seen := url.Values{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		v, _ := url.ParseQuery(string(b))
		for k, vs := range v {
			seen[k] = vs
		}
		w.Header().Set("Request-Id", "req_test")
		_, _ = w.Write([]byte(`{"id":"price_xxx","object":"price","currency":"usd","unit_amount":1900}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	desc := pricing.MustGetDescriptor(pricing.PlanStarter, pricing.PeriodMonthly, pricing.TierDeveloped)
	p, err := billingstripe.CreatePrice(context.Background(), c, "prod_starter", desc)
	require.NoError(t, err)
	require.Equal(t, "price_xxx", p.ID)

	require.Equal(t, "exclusive", seen.Get("currency_options[aud][tax_behavior]"))
	require.Equal(t, "2900", seen.Get("currency_options[aud][unit_amount]"))
	// Baseline currency (usd) is set as top-level Currency/UnitAmount on the
	// Price object — Stripe rejects a currency_options entry for the baseline.
	require.Empty(t, seen.Get("currency_options[usd][unit_amount]"))
	require.Equal(t, "1900", seen.Get("unit_amount"))
	require.Equal(t, "usd", seen.Get("currency"))
	require.Equal(t, "month", seen.Get("recurring[interval]"))
	require.Equal(t, "starter", seen.Get("metadata[plan]"))
	require.Equal(t, "monthly", seen.Get("metadata[period]"))
	require.Equal(t, "developed", seen.Get("metadata[tier]"))
	require.Equal(t, "mark8ly_starter_monthly_developed_v1", seen.Get("lookup_key"))
}

func TestCreatePrice_PPPTierOmitsCurrencyOptions(t *testing.T) {
	seen := url.Values{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		v, _ := url.ParseQuery(string(b))
		for k, vs := range v {
			seen[k] = vs
		}
		_, _ = w.Write([]byte(`{"id":"price_ppp_x","currency":"inr","unit_amount":99900}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	// Use MustGetDescriptor to get the INR starter monthly descriptor.
	desc := pricing.MustGetDescriptor(pricing.PlanStarter, pricing.PeriodMonthly, pricing.TierPPP)
	_, err := billingstripe.CreatePrice(context.Background(), c, "prod_starter", desc)
	require.NoError(t, err)

	// PPP tier must not emit developed currency_options (no usd currency_options key).
	require.Empty(t, seen.Get("currency_options[usd][unit_amount]"))

	// Verify recurring interval reflects monthly period.
	require.Equal(t, "month", seen.Get("recurring[interval]"))
}

func TestCreatePrice_AnnualPeriodSetsIntervalYear(t *testing.T) {
	seen := url.Values{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		v, _ := url.ParseQuery(string(b))
		for k, vs := range v {
			seen[k] = vs
		}
		_, _ = w.Write([]byte(`{"id":"price_yr"}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	desc := pricing.MustGetDescriptor(pricing.PlanPro, pricing.PeriodAnnual, pricing.TierDeveloped)
	_, err := billingstripe.CreatePrice(context.Background(), c, "prod_pro", desc)
	require.NoError(t, err)
	require.Equal(t, "year", seen.Get("recurring[interval]"))
}

func TestFindPriceByLookupKey_MissReturnsErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	_, err := billingstripe.FindPriceByLookupKey(context.Background(), c, "missing")
	require.True(t, errors.Is(err, billingstripe.ErrNotFound))
}

func TestFindPriceByLookupKey_EscapesQueryParam(t *testing.T) {
	var seenPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.RequestURI()
		_, _ = w.Write([]byte(`{"data":[{"id":"price_ok","lookup_key":"k/with space"}]}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	p, err := billingstripe.FindPriceByLookupKey(context.Background(), c, "k/with space")
	require.NoError(t, err)
	require.Equal(t, "price_ok", p.ID)
	// The stripe-go SDK serializes []*string slices with indexed brackets:
	// lookup_keys[0]=<value>. The value is percent-encoded.
	require.Contains(t, seenPath, "lookup_keys[0]=k%2Fwith+space")
}
