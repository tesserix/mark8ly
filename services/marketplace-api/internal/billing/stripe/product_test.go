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

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

func TestCreateProduct_SendsMetadataPlan(t *testing.T) {
	var seenValues url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seenValues, _ = url.ParseQuery(string(b))
		w.Header().Set("Request-Id", "req_test")
		_, _ = w.Write([]byte(`{"id":"prod_abc","name":"Mark8ly starter","metadata":{"plan":"starter"},"active":true}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	p, err := billingstripe.CreateProduct(context.Background(), c, "Mark8ly starter", "starter", "product:starter")
	require.NoError(t, err)
	require.Equal(t, "prod_abc", p.ID)
	require.Equal(t, "starter", seenValues.Get("metadata[plan]"))
	require.Equal(t, "Mark8ly starter", seenValues.Get("name"))
}

func TestFindProductByMetadata_MissReturnsErrNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	_, err := billingstripe.FindProductByMetadata(context.Background(), c, "starter")
	require.True(t, errors.Is(err, billingstripe.ErrNotFound))
}

func TestFindProductByMetadata_MatchesByPlanField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
            {"id":"prod_1","metadata":{"plan":"starter"}},
            {"id":"prod_2","metadata":{"plan":"studio"}}
        ]}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	p, err := billingstripe.FindProductByMetadata(context.Background(), c, "studio")
	require.NoError(t, err)
	require.Equal(t, "prod_2", p.ID)
}
