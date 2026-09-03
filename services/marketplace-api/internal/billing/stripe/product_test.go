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
