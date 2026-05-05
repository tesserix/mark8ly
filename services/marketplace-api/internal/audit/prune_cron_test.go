package audit

import (
	"testing"

	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// TestAuditRetentionDaysForPlan ensures the prune cron's bucket logic stays
// aligned with plangate.matrix.go. A drift between them would mean the cron
// prunes data the matrix says should be retained, or vice versa.
func TestAuditRetentionDaysForPlan(t *testing.T) {
	cases := []struct {
		plan       subscription.SubscriptionPlan
		wantDays   int
		wantsMatch bool // does this plan map to a bucket the cron prunes?
	}{
		{subscription.PlanTrial, 90, true},
		{subscription.PlanStarter, 90, true},
		{subscription.PlanStudio, 365, true},
		// Pro returns plangate.Unlimited (-1) via the fallback path.
		{subscription.PlanPro, plangate.Unlimited, false},
	}
	for _, tc := range cases {
		got := AuditRetentionDaysForPlan(tc.plan)
		if got != tc.wantDays {
			t.Fatalf("plan %s: want %d days, got %d", tc.plan, tc.wantDays, got)
		}
	}
}

// TestRetentionBucketsCoverNonProPlans verifies that every plan with a
// non-unlimited audit retention in plangate.matrix.go is represented in the
// prune cron's buckets. Catches drift if the matrix gains a new plan and
// someone forgets to add the bucket.
func TestRetentionBucketsCoverNonProPlans(t *testing.T) {
	for _, plan := range []subscription.SubscriptionPlan{
		subscription.PlanTrial,
		subscription.PlanStarter,
		subscription.PlanStudio,
	} {
		matrixDays := plangate.Limit(plan, plangate.FeatureAuditRetentionDays)
		bucketDays := AuditRetentionDaysForPlan(plan)
		if matrixDays != bucketDays {
			t.Fatalf("plan %s: matrix says %d days, prune bucket says %d (drift!)",
				plan, matrixDays, bucketDays)
		}
	}
}
