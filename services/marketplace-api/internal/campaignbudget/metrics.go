package campaignbudget

import "github.com/prometheus/client_golang/prometheus"

var (
	// SentTotal counts campaign emails successfully reserved against budget.
	SentTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "campaign_email_sent_total",
			Help: "Count of campaign emails successfully reserved against budget.",
		},
		[]string{"store_id"},
	)
	// BudgetExhaustedTotal counts campaign-send attempts rejected for budget exhaustion.
	BudgetExhaustedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "campaign_email_budget_exhausted_total",
			Help: "Count of campaign-send attempts rejected for budget exhaustion.",
		},
		[]string{"plan"},
	)
	// TrialRampAppliedTotal counts trial-ramp transitions applied (day 3→4 or 7→8).
	TrialRampAppliedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "campaign_email_trial_ramp_applied_total",
			Help: "Count of trial-ramp transitions applied (day 3→4 or 7→8).",
		},
		[]string{"day"},
	)
	// PlanRecomputeWarningTotal counts times plan-change recomputation hit a Negotiated plan.
	PlanRecomputeWarningTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "campaign_email_plan_recompute_negotiated_total",
			Help: "Times plan-change recomputation hit a Negotiated plan and skipped.",
		},
	)
)

// MustRegisterMetrics registers all campaignbudget Prometheus metrics with reg.
// Panics on duplicate registration — call once at startup.
func MustRegisterMetrics(reg prometheus.Registerer) {
	reg.MustRegister(SentTotal, BudgetExhaustedTotal, TrialRampAppliedTotal, PlanRecomputeWarningTotal)
}
