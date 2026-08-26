package dunning

import "github.com/prometheus/client_golang/prometheus"

// WrapPrometheusCounter wraps a *prometheus.Counter so it satisfies
// CounterIncrementer. Use this in main.go to pass
// metrics.DunningSuppressedRefundWindowTotal directly to NewStepDailyLadder.
func WrapPrometheusCounter(c prometheus.Counter) CounterIncrementer {
	return prometheusCounter{c: c}
}

// WrapPrometheusCounterVec wraps a *prometheus.CounterVec so it satisfies
// CounterVecIncrementer. Use this in main.go to pass
// metrics.DunningEmailsSentTotal / metrics.PaymentActionRemindersSentTotal
// directly to NewSendDunningEmails / NewSendPaymentActionReminders.
func WrapPrometheusCounterVec(cv *prometheus.CounterVec) CounterVecIncrementer {
	return &prometheusCounterVec{cv: cv}
}

// prometheusCounter adapts a prometheus.Counter to CounterIncrementer.
type prometheusCounter struct{ c prometheus.Counter }

func (p prometheusCounter) Inc() { p.c.Inc() }

// prometheusCounterVec adapts a *prometheus.CounterVec to CounterVecIncrementer.
type prometheusCounterVec struct{ cv *prometheus.CounterVec }

func (p *prometheusCounterVec) WithDay(label string) CounterIncrementer {
	return prometheusCounter{c: p.cv.WithLabelValues(label)}
}

// SkipCounter counts billing emails that were deliberately NOT sent,
// labeled by template and reason. Defined here at the point of use rather
// than in the metrics package so the crons stay testable with a stub.
type SkipCounter interface {
	WithTemplateReason(template, reason string) CounterIncrementer
}

// WrapPrometheusSkipCounter adapts a two-label *prometheus.CounterVec to
// SkipCounter. Use with metrics.BillingEmailsSkippedTotal in main.go.
func WrapPrometheusSkipCounter(cv *prometheus.CounterVec) SkipCounter {
	return &prometheusSkipCounter{cv: cv}
}

type prometheusSkipCounter struct{ cv *prometheus.CounterVec }

func (p *prometheusSkipCounter) WithTemplateReason(template, reason string) CounterIncrementer {
	return prometheusCounter{c: p.cv.WithLabelValues(template, reason)}
}
