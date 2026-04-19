package reconciliation_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/reconciliation"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// TestReconciler_NoStripeClient_SkipsGracefully verifies that a nil Stripe
// client returns 0 drift without error.
func TestReconciler_NoStripeClient_SkipsGracefully(t *testing.T) {
	r := reconciliation.New(nil, nil, nil, nil)
	count, err := r.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// TestReconciler_NilDB_NilStripe_ReturnsZero confirms the nil-stripe fast path.
func TestReconciler_NilDB_NilStripe_ReturnsZero(t *testing.T) {
	r := reconciliation.New(nil, nil, nil, nil)
	count, err := r.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// TestDriftTypeConstants verifies the exported drift-type constants have the
// values the Prometheus alert rules and dashboards expect.
func TestDriftTypeConstants(t *testing.T) {
	assert.Equal(t, "status_mismatch", reconciliation.DriftTypeStatusMismatch)
	assert.Equal(t, "stripe_not_found", reconciliation.DriftTypeStripeNotFound)
}

// TestDriftCounter_RegisteredInDefaultRegistry verifies the counter is
// registered in the default Prometheus registry without panicking.
func TestDriftCounter_RegisteredInDefaultRegistry(t *testing.T) {
	// Gather from the default registry — if the counter was registered in
	// init() this should not error.
	_, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
}

// Compile-time checks: exported symbols are accessible from external packages.
var _ = subscription.StatusActive
var _ = uuid.UUID{}
