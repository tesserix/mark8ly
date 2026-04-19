package stripe

import (
	"context"
	"errors"
	"net/http"

	sdk "github.com/stripe/stripe-go/v82"
)

// Coupon is our sanitised representation of a Stripe Coupon object.
type Coupon struct {
	ID               string `json:"id"`
	PercentOff       *float64 `json:"percent_off,omitempty"`
	AmountOff        *int64   `json:"amount_off,omitempty"`
	Currency         string   `json:"currency,omitempty"`
	DurationInMonths *int64   `json:"duration_in_months,omitempty"`
	Duration         string   `json:"duration"`
	MaxRedemptions   *int64   `json:"max_redemptions,omitempty"`
	Valid            bool     `json:"valid"`
}

// CreateCouponInput holds the parameters for creating a Stripe Coupon.
type CreateCouponInput struct {
	// ID is the Stripe coupon ID to create (must be unique in Stripe).
	// If empty, Stripe auto-generates one.
	ID               string
	PercentOff       *float64 // 0–100; mutually exclusive with AmountOff
	AmountOff        *int64   // minor units; mutually exclusive with PercentOff
	Currency         string   // required when AmountOff is set
	Duration         string   // "once" | "repeating" | "forever"
	DurationInMonths *int64   // required when Duration == "repeating"
	MaxRedemptions   *int64
	Name             string
	IdempotencyKey   string
}

// CreateCoupon creates a new Stripe Coupon.
func CreateCoupon(ctx context.Context, c *Client, in CreateCouponInput) (*Coupon, error) {
	params := &sdk.CouponCreateParams{}
	if in.ID != "" {
		params.ID = sdk.String(in.ID)
	}
	if in.PercentOff != nil {
		params.PercentOff = in.PercentOff
	}
	if in.AmountOff != nil {
		params.AmountOff = in.AmountOff
		if in.Currency != "" {
			params.Currency = sdk.String(in.Currency)
		}
	}
	if in.Duration != "" {
		params.Duration = sdk.String(in.Duration)
	}
	if in.DurationInMonths != nil {
		params.DurationInMonths = in.DurationInMonths
	}
	if in.MaxRedemptions != nil {
		params.MaxRedemptions = in.MaxRedemptions
	}
	if in.Name != "" {
		params.Name = sdk.String(in.Name)
	}
	if in.IdempotencyKey != "" {
		params.SetIdempotencyKey(in.IdempotencyKey)
	}

	cu, err := c.sdk.V1Coupons.Create(ctx, params)
	if err != nil {
		return nil, toAPIError(err)
	}
	return mapCoupon(cu), nil
}

// GetCoupon retrieves a Stripe Coupon by ID.
// Returns ErrNotFound when the Stripe API returns HTTP 404.
func GetCoupon(ctx context.Context, c *Client, id string) (*Coupon, error) {
	cu, err := c.sdk.V1Coupons.Retrieve(ctx, id, nil)
	if err != nil {
		var se *sdk.Error
		if errors.As(err, &se) && se.HTTPStatusCode == http.StatusNotFound {
			return nil, ErrNotFound
		}
		return nil, toAPIError(err)
	}
	return mapCoupon(cu), nil
}

// AttachCoupon attaches a Stripe Coupon to an existing Subscription by setting
// it as the sole discount via the Discounts slice (SDK v82 pattern).
// Idempotent: re-attaching the same coupon is a no-op on Stripe's side.
func AttachCoupon(ctx context.Context, c *Client, subID, couponID string) error {
	params := &sdk.SubscriptionUpdateParams{
		Discounts: []*sdk.SubscriptionUpdateDiscountParams{
			{Coupon: sdk.String(couponID)},
		},
	}
	if _, err := c.sdk.V1Subscriptions.Update(ctx, subID, params); err != nil {
		return toAPIError(err)
	}
	return nil
}

// DetachCoupon removes any active coupon from a Stripe Subscription by
// supplying an empty Discounts slice, which clears all discounts.
func DetachCoupon(ctx context.Context, c *Client, subID string) error {
	params := &sdk.SubscriptionUpdateParams{
		Discounts: []*sdk.SubscriptionUpdateDiscountParams{},
	}
	if _, err := c.sdk.V1Subscriptions.Update(ctx, subID, params); err != nil {
		return toAPIError(err)
	}
	return nil
}

func mapCoupon(cu *sdk.Coupon) *Coupon {
	out := &Coupon{
		ID:       cu.ID,
		Duration: string(cu.Duration),
		Valid:    cu.Valid,
	}
	if cu.PercentOff != 0 {
		v := cu.PercentOff
		out.PercentOff = &v
	}
	if cu.AmountOff != 0 {
		v := cu.AmountOff
		out.AmountOff = &v
		out.Currency = string(cu.Currency)
	}
	if cu.DurationInMonths != 0 {
		v := cu.DurationInMonths
		out.DurationInMonths = &v
	}
	if cu.MaxRedemptions != 0 {
		v := cu.MaxRedemptions
		out.MaxRedemptions = &v
	}
	return out
}

// CouponIdempotencyKey returns a stable idempotency key for coupon creation.
func CouponIdempotencyKey(promoCodeID string) string {
	return "coupon:" + promoCodeID
}

// SubscriptionDiscountIdempotencyKey returns a stable idempotency key for
// attaching a coupon to a subscription (§7.3 pattern B).
func SubscriptionDiscountIdempotencyKey(subID, couponID string) string {
	return "sub-discount:" + subID + ":" + couponID
}
