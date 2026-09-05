package dispatch

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func ts(t *testing.T, s string) *time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	require.NoError(t, err)
	return &v
}

// TestDecidePeriodTransitions covers the whole decision surface of
// customer.subscription.updated: the two cancel-flag directions, a rolled
// billing period, and — the case that matters most — a webhook that carries
// identical values and must emit nothing.
func TestDecidePeriodTransitions(t *testing.T) {
	jan := ts(t, "2026-01-01T00:00:00Z")
	feb := ts(t, "2026-02-01T00:00:00Z")

	cases := []struct {
		name   string
		before subscriptionPeriodState
		after  subscriptionPeriodState
		want   []string
	}{
		{
			name:   "cancel flag set schedules a cancellation",
			before: subscriptionPeriodState{PeriodStart: jan, CancelAtPeriodEnd: false},
			after:  subscriptionPeriodState{PeriodStart: jan, CancelAtPeriodEnd: true},
			want:   []string{ActionCancellationScheduled},
		},
		{
			name:   "cancel flag cleared reverses a cancellation",
			before: subscriptionPeriodState{PeriodStart: jan, CancelAtPeriodEnd: true},
			after:  subscriptionPeriodState{PeriodStart: jan, CancelAtPeriodEnd: false},
			want:   []string{ActionCancellationReversed},
		},
		{
			name:   "period start moving forward rolls the period",
			before: subscriptionPeriodState{PeriodStart: jan},
			after:  subscriptionPeriodState{PeriodStart: feb},
			want:   []string{ActionPeriodRolled},
		},
		{
			name:   "first period start ever recorded rolls the period",
			before: subscriptionPeriodState{PeriodStart: nil},
			after:  subscriptionPeriodState{PeriodStart: jan},
			want:   []string{ActionPeriodRolled},
		},
		{
			name:   "identical values emit nothing",
			before: subscriptionPeriodState{PeriodStart: jan, CancelAtPeriodEnd: true},
			after:  subscriptionPeriodState{PeriodStart: jan, CancelAtPeriodEnd: true},
			want:   nil,
		},
		{
			name:   "absent period start in the payload emits nothing",
			before: subscriptionPeriodState{PeriodStart: jan},
			after:  subscriptionPeriodState{PeriodStart: nil},
			want:   nil,
		},
		{
			name:   "period start moving backwards emits nothing",
			before: subscriptionPeriodState{PeriodStart: feb},
			after:  subscriptionPeriodState{PeriodStart: jan},
			want:   nil,
		},
		{
			name:   "a roll and a cancellation in one webhook emit both, cancel first",
			before: subscriptionPeriodState{PeriodStart: jan, CancelAtPeriodEnd: false},
			after:  subscriptionPeriodState{PeriodStart: feb, CancelAtPeriodEnd: true},
			want:   []string{ActionCancellationScheduled, ActionPeriodRolled},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, decidePeriodTransitions(tc.before, tc.after))
		})
	}
}

// TestPeriodTransitionActionsAreDistinct pins the requirement that scheduling
// a cancellation and reversing one never collapse into a single action string
// — the reversal is exactly what #701's save offer produces, and an audit
// trail that cannot tell the two apart is useless for a billing dispute.
func TestPeriodTransitionActionsAreDistinct(t *testing.T) {
	require.NotEqual(t, ActionCancellationScheduled, ActionCancellationReversed)
	require.Equal(t, "subscription.cancellation_scheduled", ActionCancellationScheduled)
	require.Equal(t, "subscription.cancellation_reversed", ActionCancellationReversed)
	require.Equal(t, "subscription.period_rolled", ActionPeriodRolled)
}

// TestPeriodTransitionEvent_NilEmitterSafe proves the event is constructed and
// handed to a nil *audit.Emitter without panicking — d.emitter is nil in every
// test wiring and in any deployment that opts out of auditing.
func TestPeriodTransitionEvent_NilEmitterSafe(t *testing.T) {
	d := &Dispatcher{emitter: nil}
	require.NotPanics(t, func() {
		d.emitPeriodTransitions(
			periodTransitionContext{Customer: "cus_123"},
			subscriptionPeriodState{PeriodStart: nil, CancelAtPeriodEnd: false},
			subscriptionPeriodState{PeriodStart: ts(t, "2026-01-01T00:00:00Z"), CancelAtPeriodEnd: true},
		)
	})
}
