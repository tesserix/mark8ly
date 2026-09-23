package cancel_test

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/cancel"
)

// These cover the refusal paths, which are the ones that matter most and the
// ones that need no database: every refusal happens before any write, so the
// Service below is built with a nil *gorm.DB on purpose. A path that reached
// the DB would panic rather than quietly pass.

// stubRepo answers one subscription and nothing else. Any other Repository
// call panics on the nil embedded interface.
type stubRepo struct {
	subscription.Repository
	sub *subscription.StoreSubscription
}

func (s *stubRepo) GetByStoreID(_ context.Context, _ *gorm.DB, _, _ uuid.UUID) (*subscription.StoreSubscription, error) {
	return s.sub, nil
}

// failingCanceller reports that Stripe could not be reached, and records
// whether it was called at all.
type failingCanceller struct {
	cancelCalls int
	resumeCalls int
}

func (f *failingCanceller) CancelAtPeriodEnd(_ context.Context, _ string) (cancel.StripeState, error) {
	f.cancelCalls++
	return cancel.StripeState{}, errors.New("stripe: connection refused")
}

func (f *failingCanceller) Resume(_ context.Context, _ string) (cancel.StripeState, error) {
	f.resumeCalls++
	return cancel.StripeState{}, errors.New("stripe: connection refused")
}

func billedSub(status subscription.SubscriptionStatus) *subscription.StoreSubscription {
	stripeID := "sub_live_abc"
	periodEnd := time.Now().Add(20 * 24 * time.Hour).UTC()
	return &subscription.StoreSubscription{
		ID:                   uuid.New(),
		TenantID:             uuid.New(),
		StoreID:              uuid.New(),
		StripeCustomerID:     "cus_live_abc",
		StripeSubscriptionID: &stripeID,
		Status:               status,
		Plan:                 subscription.PlanStudio,
		CurrentPeriodEnd:     &periodEnd,
	}
}

func inputFor(sub *subscription.StoreSubscription) cancel.Input {
	return cancel.Input{
		TenantID: sub.TenantID,
		StoreID:  sub.StoreID,
		Actor:    "user:" + uuid.NewString(),
	}
}

// TestCancel_RefusesWhenStripeIsNotWired — a row Stripe is billing must never
// be cancelled locally by a process that cannot stop the charging. Telling a
// merchant they have cancelled while Stripe keeps billing them is the defect
// this whole path exists to prevent.
func TestCancel_RefusesWhenStripeIsNotWired(t *testing.T) {
	sub := billedSub(subscription.StatusActive)
	svc := cancel.NewService(nil, &stubRepo{sub: sub}, nil, slog.Default())

	_, err := svc.Cancel(context.Background(), inputFor(sub))

	require.ErrorIs(t, err, cancel.ErrStripeNotWired)
}

// TestCancel_RefusesWhenStripeFails — a Stripe outage fails the request
// rather than recording a cancellation Stripe does not know about.
func TestCancel_RefusesWhenStripeFails(t *testing.T) {
	sub := billedSub(subscription.StatusActive)
	stripe := &failingCanceller{}
	svc := cancel.NewService(nil, &stubRepo{sub: sub}, nil, slog.Default()).WithStripe(stripe)

	_, err := svc.Cancel(context.Background(), inputFor(sub))

	require.ErrorIs(t, err, cancel.ErrStripeUnavailable)
	require.Equal(t, 1, stripe.cancelCalls, "Stripe must be attempted before anything local")
}

// TestSaveOffer_RefusesWhenStripeFails — accepting the save offer tells the
// merchant their subscription stays. It must not be said while Stripe still
// intends to stop billing at the period end.
func TestSaveOffer_RefusesWhenStripeFails(t *testing.T) {
	sub := billedSub(subscription.StatusCancelScheduled)
	stripe := &failingCanceller{}
	svc := cancel.NewService(nil, &stubRepo{sub: sub}, nil, slog.Default()).WithStripe(stripe)

	in := inputFor(sub)
	in.AcceptSaveOffer = true
	_, err := svc.Cancel(context.Background(), in)

	require.ErrorIs(t, err, cancel.ErrStripeUnavailable)
	require.Equal(t, 1, stripe.resumeCalls)
}

// TestCancel_StatusGuardRunsBeforeStripe — an expired subscription is
// refused without a Stripe round-trip.
func TestCancel_StatusGuardRunsBeforeStripe(t *testing.T) {
	sub := billedSub(subscription.StatusExpired)
	stripe := &failingCanceller{}
	svc := cancel.NewService(nil, &stubRepo{sub: sub}, nil, slog.Default()).WithStripe(stripe)

	_, err := svc.Cancel(context.Background(), inputFor(sub))

	require.ErrorIs(t, err, cancel.ErrNotCancellable)
	require.Zero(t, stripe.cancelCalls, "an uncancellable status must not reach Stripe")
}

// TestStripeCanceller_CannotCreditOrRefund keeps §15.1 (prospective-only)
// enforced now that this package does hold a Stripe seam.
//
// It replaces the older structural claim that cancel.Service takes no Stripe
// client at all — which was true, and was also why cancellation never
// reached Stripe. The real invariant was never "no Stripe": it is that this
// flow can schedule and reverse a cancellation and can do nothing else. Two
// methods, neither of which moves money backwards.
func TestStripeCanceller_CannotCreditOrRefund(t *testing.T) {
	require.ElementsMatch(t, []string{"CancelAtPeriodEnd", "Resume"}, stripeCancellerMethods(),
		"§15.1 is prospective-only: this seam may not grow a credit, refund or proration method")
}

// stripeCancellerMethods lists the method set of the Stripe seam this package
// holds. Shared with service_test.go, which asserts the same invariant from
// the save offer's side.
func stripeCancellerMethods() []string {
	iface := reflect.TypeOf((*cancel.StripeCanceller)(nil)).Elem()
	out := make([]string, 0, iface.NumMethod())
	for i := range iface.NumMethod() {
		out = append(out, iface.Method(i).Name)
	}
	return out
}
