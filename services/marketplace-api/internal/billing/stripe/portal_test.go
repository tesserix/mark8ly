package stripe_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

func TestCreatePortalSession_5MinBucketIdempotency(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"ps_x","url":"https://billing.stripe.com/ps_x"}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	// Bucket 5706666 covers [5706666*300, 5706667*300) = [1711999800, 1712000100).
	// Pick +50s and +250s within that window — both land in bucket 5706666.
	base := int64(5706666) * 300
	in1 := billingstripe.PortalInput{
		StoreID: "store-1", CustomerID: "cus_1", ReturnURL: "https://admin/",
		Now: time.Unix(base+50, 0),
	}
	a, err := billingstripe.CreatePortalSession(context.Background(), c, in1)
	require.NoError(t, err)

	in2 := in1
	in2.Now = time.Unix(base+250, 0) // still in bucket 5706666
	b, err := billingstripe.CreatePortalSession(context.Background(), c, in2)
	require.NoError(t, err)
	require.Equal(t, a.IdempotencyKey, b.IdempotencyKey)

	in3 := in1
	in3.Now = time.Unix(base+350, 0) // crosses into bucket 5706667
	c2, err := billingstripe.CreatePortalSession(context.Background(), c, in3)
	require.NoError(t, err)
	require.NotEqual(t, a.IdempotencyKey, c2.IdempotencyKey)
}
