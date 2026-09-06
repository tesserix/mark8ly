package stripe

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	sdk "github.com/stripe/stripe-go/v82"
)

// Coupon is our sanitised representation of a Stripe Coupon object.
type Coupon struct {
	ID               string   `json:"id"`
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

// maxSubscriptionDiscounts is Stripe's cap on the size of a subscription's
// discounts array. Enforced here so a full subscription is reported with its
// id rather than as an opaque API rejection.
const maxSubscriptionDiscounts = 20

// ErrTooManyDiscounts is what a TooManyDiscountsError unwraps to, so callers
// can branch on the condition without depending on the concrete type.
var ErrTooManyDiscounts = errors.New("stripe: subscription discount limit reached")

// TooManyDiscountsError reports a subscription that already carries Stripe's
// maximum number of discounts, naming the subscription and the count found.
type TooManyDiscountsError struct {
	SubscriptionID string
	Count          int
}

func (e *TooManyDiscountsError) Error() string {
	return fmt.Sprintf("stripe: subscription %s already carries %d discounts (Stripe's maximum is %d)",
		e.SubscriptionID, e.Count, maxSubscriptionDiscounts)
}

func (e *TooManyDiscountsError) Unwrap() error { return ErrTooManyDiscounts }

// subscriptionDiscounts retrieves the subscription's current discounts with
// the array expanded. Expansion is required, not an optimisation: unexpanded,
// Stripe returns bare discount ids and the coupon behind each one — the only
// thing our callers can match on — is not present.
func subscriptionDiscounts(ctx context.Context, c *Client, subID string) ([]*sdk.Discount, error) {
	params := &sdk.SubscriptionRetrieveParams{}
	params.AddExpand("discounts")
	s, err := c.sdk.V1Subscriptions.Retrieve(ctx, subID, params)
	if err != nil {
		return nil, toAPIError(err)
	}
	return s.Discounts, nil
}

// discountFromCoupon reports whether d is a discount created from couponID.
// One definition for the three callers below, so "is our coupon on this
// subscription" cannot come to mean different things in the add, the remove
// and the read.
func discountFromCoupon(d *sdk.Discount, couponID string) bool {
	return d != nil && d.Coupon != nil && d.Coupon.ID == couponID
}

// SubscriptionHasDiscount reports whether the subscription already carries a
// discount created from couponID.
//
// AddSubscriptionDiscount treats an already-attached coupon as a no-op, but it
// reports that no-op and a real attachment identically — both return nil. A
// caller that must tell "we applied it" from "it was already there", which the
// tenant-discount fan-out's per-store report is, needs the question asked
// separately. Asking it here rather than widening AddSubscriptionDiscount's
// return leaves that helper and its three call sites in internal/promo alone.
//
// The answer is a snapshot: nothing stops the array changing between this read
// and a following add, and it is not meant to. AddSubscriptionDiscount stays
// idempotent on its own, so the worst outcome of a lost race is an outcome
// label that reads "applied" for a coupon a concurrent call had just attached.
func SubscriptionHasDiscount(ctx context.Context, c *Client, subID, couponID string) (bool, error) {
	if subID == "" || couponID == "" {
		return false, errors.New("stripe: SubscriptionHasDiscount: subscription and coupon ids are required")
	}
	existing, err := subscriptionDiscounts(ctx, c, subID)
	if err != nil {
		return false, err
	}
	for _, d := range existing {
		if discountFromCoupon(d, couponID) {
			return true, nil
		}
	}
	return false, nil
}

// reuseParams maps existing discounts to update params that reuse them.
// The Discount arm carries a discount id already on the object; the Coupon
// arm would mint a second discount from the same coupon instead.
func reuseParams(subID string, discounts []*sdk.Discount) ([]*sdk.SubscriptionUpdateDiscountParams, error) {
	// Non-nil even when empty: stripe-go encodes a nil slice as nothing at
	// all (leaving the array untouched) and a non-nil empty one as
	// "discounts=", which is what clears it.
	out := make([]*sdk.SubscriptionUpdateDiscountParams, 0, len(discounts)+1)
	for _, d := range discounts {
		if d == nil {
			continue
		}
		if d.ID == "" {
			// Writing the array back without an id we cannot name would
			// silently drop that discount — the bug this pair replaces.
			return nil, fmt.Errorf("stripe: subscription %s carries a discount with no id; refusing to rewrite its discounts", subID)
		}
		out = append(out, &sdk.SubscriptionUpdateDiscountParams{Discount: sdk.String(d.ID)})
	}
	return out, nil
}

// putSubscriptionDiscounts writes discounts back as the subscription's whole
// array.
//
// No Idempotency-Key is set, unlike the object-creating POSTs whose key
// generators live in client.go. A retried create must not make a second
// object; this update is a read-modify-write whose result depends on the
// state it just read. A key stable across calls ("this coupon on this
// subscription") would, within Stripe's 24-hour key window, replay the
// cached response for a re-add that follows a removal — skipping the update
// while reporting success. Idempotency comes from the membership check
// instead, which re-reads state on every attempt.
func putSubscriptionDiscounts(ctx context.Context, c *Client, subID string, discounts []*sdk.SubscriptionUpdateDiscountParams) error {
	params := &sdk.SubscriptionUpdateParams{Discounts: discounts}
	if _, err := c.sdk.V1Subscriptions.Update(ctx, subID, params); err != nil {
		return toAPIError(err)
	}
	return nil
}

// AddSubscriptionDiscount adds couponID to a subscription's discounts,
// preserving every discount already there — including a merchant's own promo.
//
// A coupon already on the subscription is a no-op, so the call is safe to
// retry on its own.
func AddSubscriptionDiscount(ctx context.Context, c *Client, subID, couponID string) error {
	if subID == "" || couponID == "" {
		return errors.New("stripe: AddSubscriptionDiscount: subscription and coupon ids are required")
	}

	existing, err := subscriptionDiscounts(ctx, c, subID)
	if err != nil {
		return err
	}
	for _, d := range existing {
		if discountFromCoupon(d, couponID) {
			return nil
		}
	}
	if len(existing) >= maxSubscriptionDiscounts {
		return &TooManyDiscountsError{SubscriptionID: subID, Count: len(existing)}
	}

	discounts, err := reuseParams(subID, existing)
	if err != nil {
		return err
	}
	discounts = append(discounts, &sdk.SubscriptionUpdateDiscountParams{Coupon: sdk.String(couponID)})
	return putSubscriptionDiscounts(ctx, c, subID, discounts)
}

// RemoveSubscriptionDiscount removes the discount created from couponID and
// writes the rest of the array back untouched.
//
// A coupon that is not attached is a no-op — no update is sent at all — so
// the call is safe to retry on its own. Should the same coupon somehow back
// two of the subscription's discounts, both go.
func RemoveSubscriptionDiscount(ctx context.Context, c *Client, subID, couponID string) error {
	if subID == "" || couponID == "" {
		return errors.New("stripe: RemoveSubscriptionDiscount: subscription and coupon ids are required")
	}

	existing, err := subscriptionDiscounts(ctx, c, subID)
	if err != nil {
		return err
	}

	keep := make([]*sdk.Discount, 0, len(existing))
	found := false
	for _, d := range existing {
		if discountFromCoupon(d, couponID) {
			found = true
			continue
		}
		keep = append(keep, d)
	}
	if !found {
		return nil
	}

	discounts, err := reuseParams(subID, keep)
	if err != nil {
		return err
	}
	return putSubscriptionDiscounts(ctx, c, subID, discounts)
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
