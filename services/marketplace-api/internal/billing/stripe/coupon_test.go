package stripe_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

// discountStub is one entry of an expanded `discounts` array as Stripe
// returns it: a discount object carrying its own id and the coupon it was
// created from.
type discountStub struct {
	discountID string
	couponID   string
}

func discountsJSON(ds ...discountStub) string {
	parts := make([]string, 0, len(ds))
	for _, d := range ds {
		parts = append(parts, fmt.Sprintf(
			`{"object":"discount","id":%q,"coupon":{"id":%q,"object":"coupon","valid":true}}`,
			d.discountID, d.couponID))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// subDiscountStub serves a subscription whose expanded discounts array is ds,
// records every POST body, and returns the recorded bodies via the closure.
// The POST response is deliberately minimal: these helpers return only an
// error, so nothing reads the updated subscription back.
func subDiscountStub(t *testing.T, subID string, ds ...discountStub) (*billingstripe.Client, *[]url.Values, *[]url.Values) {
	t.Helper()

	posts := &[]url.Values{}
	gets := &[]url.Values{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/subscriptions/"+subID, r.URL.Path)
		switch r.Method {
		case http.MethodGet:
			*gets = append(*gets, r.URL.Query())
			_, _ = fmt.Fprintf(w, `{"id":%q,"status":"active","discounts":%s}`, subID, discountsJSON(ds...))
		case http.MethodPost:
			require.NoError(t, r.ParseForm())
			*posts = append(*posts, r.PostForm)
			_, _ = fmt.Fprintf(w, `{"id":%q,"status":"active"}`, subID)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	t.Cleanup(srv.Close)

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)
	return c, posts, gets
}

// The regression the whole read-modify-write design exists for: an operator
// applying a platform coupon must not delete the merchant's own promo.
func TestAddSubscriptionDiscount_PreservesAnExistingMerchantPromo(t *testing.T) {
	c, posts, gets := subDiscountStub(t, "sub_1",
		discountStub{discountID: "di_merchant", couponID: "coupon_merchant_promo"})

	err := billingstripe.AddSubscriptionDiscount(context.Background(), c, "sub_1", "coupon_platform_override")
	require.NoError(t, err)

	require.Len(t, *gets, 1)
	require.Equal(t, "discounts", (*gets)[0].Get("expand[0]"),
		"the coupon behind each existing discount is only visible when discounts are expanded")

	require.Len(t, *posts, 1)
	body := (*posts)[0]
	// The merchant's discount is preserved by its DISCOUNT id (reuse), and
	// ours is added by its COUPON id (create). Both are present.
	require.Equal(t, "di_merchant", body.Get("discounts[0][discount]"))
	require.Equal(t, "coupon_platform_override", body.Get("discounts[1][coupon]"))
	require.Empty(t, body.Get("discounts[0][coupon]"))
	require.Empty(t, body.Get("discounts[2][coupon]"))
}

func TestAddSubscriptionDiscount_AlreadyPresentDoesNotDuplicate(t *testing.T) {
	c, posts, _ := subDiscountStub(t, "sub_1",
		discountStub{discountID: "di_merchant", couponID: "coupon_merchant_promo"},
		discountStub{discountID: "di_ours", couponID: "coupon_platform_override"})

	err := billingstripe.AddSubscriptionDiscount(context.Background(), c, "sub_1", "coupon_platform_override")
	require.NoError(t, err)
	require.Empty(t, *posts, "a coupon already on the subscription is a no-op, not a second update")
}

func TestAddSubscriptionDiscount_RefusesAtStripesTwentyDiscountCap(t *testing.T) {
	full := make([]discountStub, 0, 20)
	for i := range 20 {
		full = append(full, discountStub{
			discountID: fmt.Sprintf("di_%d", i),
			couponID:   fmt.Sprintf("coupon_%d", i),
		})
	}
	c, posts, _ := subDiscountStub(t, "sub_full", full...)

	err := billingstripe.AddSubscriptionDiscount(context.Background(), c, "sub_full", "coupon_platform_override")
	require.Error(t, err)
	require.ErrorIs(t, err, billingstripe.ErrTooManyDiscounts)
	require.Contains(t, err.Error(), "sub_full", "the message must say which subscription is full")
	require.Empty(t, *posts, "the cap is refused locally, not by letting Stripe reject the update")
}

func TestRemoveSubscriptionDiscount_RemovesOnlyTheNamedCoupon(t *testing.T) {
	c, posts, _ := subDiscountStub(t, "sub_1",
		discountStub{discountID: "di_merchant", couponID: "coupon_merchant_promo"},
		discountStub{discountID: "di_ours", couponID: "coupon_platform_override"},
		discountStub{discountID: "di_other", couponID: "coupon_other"})

	err := billingstripe.RemoveSubscriptionDiscount(context.Background(), c, "sub_1", "coupon_platform_override")
	require.NoError(t, err)

	require.Len(t, *posts, 1)
	body := (*posts)[0]
	require.Equal(t, "di_merchant", body.Get("discounts[0][discount]"))
	require.Equal(t, "di_other", body.Get("discounts[1][discount]"))
	require.Empty(t, body.Get("discounts[2][discount]"), "exactly the two survivors are written back")
	require.Empty(t, body.Get("discounts[0][coupon]"))
	require.Empty(t, body.Get("discounts[1][coupon]"))
}

func TestRemoveSubscriptionDiscount_AbsentIsNoOp(t *testing.T) {
	c, posts, _ := subDiscountStub(t, "sub_1",
		discountStub{discountID: "di_merchant", couponID: "coupon_merchant_promo"})

	err := billingstripe.RemoveSubscriptionDiscount(context.Background(), c, "sub_1", "coupon_platform_override")
	require.NoError(t, err)
	require.Empty(t, *posts, "a coupon that is not attached must not provoke an update that rewrites the array")
}

func TestRemoveSubscriptionDiscount_LastDiscountClearsTheArray(t *testing.T) {
	c, posts, _ := subDiscountStub(t, "sub_1",
		discountStub{discountID: "di_ours", couponID: "coupon_platform_override"})

	err := billingstripe.RemoveSubscriptionDiscount(context.Background(), c, "sub_1", "coupon_platform_override")
	require.NoError(t, err)

	require.Len(t, *posts, 1)
	body := (*posts)[0]
	// stripe-go encodes a non-nil empty slice as `discounts=`, which is how
	// Stripe is told "no discounts" rather than "field omitted".
	require.Equal(t, []string{""}, body["discounts"])
}

func TestSubscriptionDiscountHelpers_MapStripeErrorsThroughAPIError(t *testing.T) {
	t.Run("retrieve failure", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Request-Id", "req_ret")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error","code":"resource_missing","message":"No such subscription: sub_gone"}}`))
		}))
		defer srv.Close()
		c := billingstripe.New("sk_test_x")
		c.SetBaseURLForTesting(srv.URL)

		err := billingstripe.AddSubscriptionDiscount(context.Background(), c, "sub_gone", "coupon_x")
		var apiErr *billingstripe.APIError
		require.True(t, errors.As(err, &apiErr))
		require.Equal(t, http.StatusNotFound, apiErr.HTTPStatus)
		require.Equal(t, "resource_missing", apiErr.Code)
		require.NotContains(t, err.Error(), "No such subscription")
	})

	t.Run("update failure", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				_, _ = fmt.Fprintf(w, `{"id":"sub_1","status":"active","discounts":%s}`,
					discountsJSON(discountStub{discountID: "di_ours", couponID: "coupon_x"}))
				return
			}
			w.Header().Set("Request-Id", "req_upd")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error","code":"coupon_expired","message":"secret detail"}}`))
		}))
		defer srv.Close()
		c := billingstripe.New("sk_test_x")
		c.SetBaseURLForTesting(srv.URL)

		err := billingstripe.RemoveSubscriptionDiscount(context.Background(), c, "sub_1", "coupon_x")
		var apiErr *billingstripe.APIError
		require.True(t, errors.As(err, &apiErr))
		require.Equal(t, "coupon_expired", apiErr.Code)
		require.NotContains(t, err.Error(), "secret detail")
	})
}
