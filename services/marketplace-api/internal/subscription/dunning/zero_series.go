package dunning

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// PublishZeroSeries materialises one child per label value these crons can
// emit, so each counter exports a series from process start.
//
// A CounterVec with no children exports NOTHING. Every counter here is
// therefore absent from Prometheus until the first send, which for the trial
// ladder means absent until a merchant's T-15 lands — and "no series" is
// indistinguishable from "the cron is dead", "the metric was renamed" and
// "nobody is scraping this pod".
//
// That ambiguity is not hypothetical. As of 2026-09-24 the trial reminder
// cron has existed since 2026-05-05 and sent zero reminders, and answering
// "is that broken?" took reading the cron's registration, its eligibility
// query, the trial_reminders table, and the audit trail of two closed
// stores. The answer was benign — the trial crons only began running around
// 2026-09-09, by which time both stores were past every window, and the two
// live trials have their first windows in October and November. None of that
// was visible from the metric, because there was no metric.
//
// The label values come from the SAME target lists the crons iterate, so a
// new offset or dunning day is published automatically. A hand-kept list
// here would be the bug this function exists to prevent, one layer over.
//
// Called once at startup from main.go, where these counters are wired.
func PublishZeroSeries(trialReminders, scaReminders, dunningEmails *prometheus.CounterVec) {
	for _, t := range trialReminderTargets {
		trialReminders.WithLabelValues(t.OffsetKey)
	}
	for _, t := range paymentActionTargets {
		scaReminders.WithLabelValues(t.OffsetKey)
	}
	for _, t := range dunningEmailTargets {
		dunningEmails.WithLabelValues(dayLabel(t.Day))
	}
}

// dayLabel is the label the dunning-email cron stamps on its counter.
// Extracted so the zero-series publisher and the sender cannot drift into
// formatting the same day differently — which would give the series two
// spellings and make a rate() over either one quietly wrong.
func dayLabel(day int) string {
	return fmt.Sprintf("day_%d", day)
}
