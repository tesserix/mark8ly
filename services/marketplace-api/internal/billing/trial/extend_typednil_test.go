package trial_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/billing/trial"
)

// NO BUILD TAG, NO DATABASE — deliberately (#358 F3).
//
// The guard this file protects (NewExtender's reflect normalisation of a
// typed-nil StripeTrialUpdater) defends against a Critical this repo
// actually shipped two weeks ago: #288's typed-nil *gipadmin.AdminClient,
// which panicked AFTER its transaction had committed. Both of this guard's
// tests previously lived in a //go:build integration file needing a live
// TEST_DATABASE_URL, so on CI without a database the guard was proven by
// nothing at all. This one needs neither, so it runs everywhere.

// nilableUpdater exists only to be a typed nil. It is declared here rather
// than reusing the integration file's fakeUpdater precisely because this
// file must compile and run without that file's build tag.
type nilableUpdater struct{}

func (n *nilableUpdater) GetSubscription(context.Context, string) (*billingstripe.Subscription, error) {
	panic("must never be called: a typed nil must be normalised to a true nil first")
}

func (n *nilableUpdater) UpdateTrialEnd(context.Context, billingstripe.UpdateTrialEndParams) (*billingstripe.Subscription, error) {
	panic("must never be called: a typed nil must be normalised to a true nil first")
}

// A typed nil in an interface is NOT nil. Assigning a nil *stripe.Client
// into StripeTrialUpdater makes `e.Stripe != nil` TRUE, and the first method
// call panics — after the row lock has been taken and inside a transaction.
//
// The assertion is `e.Stripe == nil` and NOT require.Nil(t, e.Stripe):
// require.Nil unwraps a typed nil through its own reflection, so it reports
// success for the exact value this guard exists to reject. The old form
// passed with the reflect normalisation deleted from NewExtender, which
// means it could not fail for the reason it names. `== nil` is Go's own
// interface comparison and IS false for a typed nil (#358 F3).
func TestNewExtender_TypedNilUpdaterIsTreatedAsAbsent(t *testing.T) {
	var typedNil *nilableUpdater // nil POINTER; a non-nil INTERFACE once assigned

	e := trial.NewExtender(typedNil)
	require.True(t, e.Stripe == nil,
		"a typed-nil updater must be normalised to a true nil, or every card-backed extension panics")
}
