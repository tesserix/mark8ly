package tenantdiscount_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/billing/tenantdiscount"
)

// The checks in this file run before any database handle is touched, so they
// are ordinary unit tests: they are the reason the validation happens BEFORE
// the fan-out query rather than inside the per-store loop, where a missing
// coupon id would be reported once per store as a Stripe failure.

func TestNewService_RefusesANilAuditWriter(t *testing.T) {
	_, err := tenantdiscount.NewService(tenantdiscount.Config{
		DB:     nil,
		Stripe: newFakeStripe(),
		Audit:  nil,
	})
	require.ErrorIs(t, err, tenantdiscount.ErrNoAuditWriter)
}

// A nil *audit.Emitter assigned into the AuditWriter interface is a NON-nil
// interface, so `s.audit != nil` would be true and every store would fail at
// its EmitTx call — inside an open transaction holding a row lock. The
// Extender's NewExtender guards the same shape (trial/extend.go:198); this
// mirrors it, and refuses rather than normalising because an audit writer is
// mandatory here where a Stripe client is optional there.
func TestNewService_RefusesATypedNilAuditWriter(t *testing.T) {
	var emitter *nilAuditWriter
	_, err := tenantdiscount.NewService(tenantdiscount.Config{
		Stripe: newFakeStripe(),
		Audit:  emitter,
	})
	require.ErrorIs(t, err, tenantdiscount.ErrNoAuditWriter)
}

func TestNewService_RefusesANilStripeClient(t *testing.T) {
	_, err := tenantdiscount.NewService(tenantdiscount.Config{
		Stripe: nil,
		Audit:  &recordingAudit{},
	})
	require.ErrorIs(t, err, tenantdiscount.ErrNoStripeClient)
}

func TestApplyAndRemove_RefuseAMissingTenantOrCoupon(t *testing.T) {
	svc := newServiceForValidation(t)

	for _, tc := range []struct {
		name string
		in   tenantdiscount.Input
		want error
	}{
		{"no tenant", tenantdiscount.Input{CouponID: "co_x"}, tenantdiscount.ErrNoTenant},
		{"no coupon", tenantdiscount.Input{TenantID: uuid.New()}, tenantdiscount.ErrNoCoupon},
		{"blank coupon", tenantdiscount.Input{TenantID: uuid.New(), CouponID: "   "}, tenantdiscount.ErrNoCoupon},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Apply(context.Background(), tc.in)
			require.ErrorIs(t, err, tc.want)
			_, err = svc.Remove(context.Background(), tc.in)
			require.ErrorIs(t, err, tc.want)
		})
	}
}

// The adapter is what makes the port reachable from a real Stripe client, and
// a port nothing can implement is a port that never ships. This only asserts
// the three methods exist with the signatures the interface names — the
// delegation itself is exercised against a stub server in the stripe package's
// own tests.
func TestStripeAdapter_SatisfiesThePort(t *testing.T) {
	var _ tenantdiscount.StripeDiscounts = &tenantdiscount.StripeAdapter{C: billingstripe.New("sk_test_x")}
}

func newServiceForValidation(t *testing.T) *tenantdiscount.Service {
	t.Helper()
	svc, err := tenantdiscount.NewService(tenantdiscount.Config{
		DB:     nil, // never reached: validation refuses before the fan-out query
		Stripe: newFakeStripe(),
		Audit:  &recordingAudit{},
	})
	require.NoError(t, err)
	return svc
}
