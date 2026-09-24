package dunning

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func newVec(name, label string) *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{Name: name}, []string{label})
}

// A CounterVec with no children exports NOTHING, so every one of these
// counters is absent from Prometheus until its first send — and for the trial
// ladder that is a merchant's T-15, potentially weeks away on a completely
// healthy deployment. Absent is indistinguishable from a dead cron.
func TestPublishZeroSeries_MaterialisesEveryLabelTheCronsCanEmit(t *testing.T) {
	trial := newVec("trial_reminders_total", "offset")
	sca := newVec("sca_reminders_total", "offset")
	dunning := newVec("dunning_emails_total", "day")

	for name, vec := range map[string]*prometheus.CounterVec{
		"trial": trial, "sca": sca, "dunning": dunning,
	} {
		if n := testutil.CollectAndCount(vec); n != 0 {
			t.Fatalf("%s exported %d series before publishing, want 0", name, n)
		}
	}

	PublishZeroSeries(trial, sca, dunning)

	for _, tc := range []struct {
		name string
		vec  *prometheus.CounterVec
		want int
	}{
		{"trial", trial, len(trialReminderTargets)},
		{"sca", sca, len(paymentActionTargets)},
		{"dunning", dunning, len(dunningEmailTargets)},
	} {
		if n := testutil.CollectAndCount(tc.vec); n != tc.want {
			t.Errorf("%s exported %d series, want %d", tc.name, n, tc.want)
		}
	}
}

// The published labels must be the ones the senders actually use. A zero
// series under a different spelling is worse than none: the metric looks
// alive while the real sends land on a second, invisible series.
func TestPublishZeroSeries_UsesTheSendersOwnLabels(t *testing.T) {
	trial := newVec("trial_reminders_total", "offset")
	sca := newVec("sca_reminders_total", "offset")
	dunning := newVec("dunning_emails_total", "day")
	PublishZeroSeries(trial, sca, dunning)

	for _, target := range trialReminderTargets {
		if got := testutil.ToFloat64(trial.WithLabelValues(target.OffsetKey)); got != 0 {
			t.Errorf("trial offset %q = %v, want a published zero", target.OffsetKey, got)
		}
	}
	for _, target := range paymentActionTargets {
		if got := testutil.ToFloat64(sca.WithLabelValues(target.OffsetKey)); got != 0 {
			t.Errorf("sca offset %q = %v, want a published zero", target.OffsetKey, got)
		}
	}
	// dayLabel is shared with the sender precisely so these cannot drift.
	for _, target := range dunningEmailTargets {
		if got := testutil.ToFloat64(dunning.WithLabelValues(dayLabel(target.Day))); got != 0 {
			t.Errorf("dunning day %q = %v, want a published zero", dayLabel(target.Day), got)
		}
	}
}

// Publishing must not invent series. If it did, the counts above would pass
// while Prometheus carried labels no cron will ever increment.
func TestPublishZeroSeries_PublishesNothingBeyondTheTargets(t *testing.T) {
	trial := newVec("trial_reminders_total", "offset")
	sca := newVec("sca_reminders_total", "offset")
	dunning := newVec("dunning_emails_total", "day")
	PublishZeroSeries(trial, sca, dunning)

	if n := testutil.CollectAndCount(trial); n != len(trialReminderTargets) {
		t.Errorf("trial exported %d series for %d targets", n, len(trialReminderTargets))
	}
	if len(trialReminderTargets) < 5 {
		t.Fatalf("only %d trial targets; the ladder has drifted and this test "+
			"would pass while covering almost nothing", len(trialReminderTargets))
	}
}
