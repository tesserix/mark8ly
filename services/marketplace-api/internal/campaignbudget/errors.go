// Package campaignbudget enforces per-month campaign email limits (spec §10).
// Every call site mutating the budget goes through this package; there is no
// correct path that bypasses Reserve.
package campaignbudget

import "errors"

// ErrBudgetExhausted is returned by Reserve when remaining < recipient_count.
// The HTTP layer maps this to 403 + upgrade-message copy per spec §10.1.
var ErrBudgetExhausted = errors.New("campaign email budget exhausted")

// ErrNoBudgetRow means no row exists for (store_id, current_month). Happens
// if the monthly-reset cron has not yet run for this store (e.g. first send
// of the month before 00:05 UTC, or a brand-new store signed up mid-month
// and the signup handler failed to seed the row). Treated as an upstream bug;
// the HTTP layer maps it to 500 + operator alert.
var ErrNoBudgetRow = errors.New("no campaign_email_budget row for current month")

// ErrPlanNegotiated means plangate.Limit returned plangate.Negotiated (Pro —
// "contact sales"). RecomputeLimitForPlan leaves limit_set unchanged and emits
// a warning metric for ops review.
var ErrPlanNegotiated = errors.New("plan has negotiated email ceiling — manual set required")
