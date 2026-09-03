package stripe_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

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
