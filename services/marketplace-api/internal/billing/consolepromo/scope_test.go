package consolepromo

import (
	"context"
	"testing"

	"github.com/lib/pq"
)

// These cover the scope half of a published definition — allowed_plans and
// annual_only (#795). now, ptr and mustMap come from mapper_test.go.

// scoped is a minimal definition carrying a benefit, so promo_codes_has_benefit
// is satisfied and every assertion below is about the scope alone.
func scoped(plans []string, annualOnly bool) Code {
	return Code{
		Code:               "LAUNCH50",
		TrialExtensionDays: ptr(14),
		AllowedPlans:       plans,
		AnnualOnly:         annualOnly,
	}
}

// TestMapCode_AllowedPlansReachTheRow is the point of the ingest half: before
// #795 the console could publish a scope and nothing carried it into the row.
func TestMapCode_AllowedPlansReachTheRow(t *testing.T) {
	got := mustMap(t, scoped([]string{"pro"}, false))

	want := pq.StringArray{"pro"}
	if len(got.AllowedPlans) != len(want) || got.AllowedPlans[0] != want[0] {
		t.Fatalf("AllowedPlans = %v, want %v", got.AllowedPlans, want)
	}
	if got.AnnualOnly {
		t.Fatalf("AnnualOnly = true, want false")
	}
}

// TestMapCode_AnnualOnlyReachesTheRow pins the other published field. It is a
// plain bool on both sides, so the risk is not a bad conversion but the field
// simply never being read — which is exactly what #795 was.
func TestMapCode_AnnualOnlyReachesTheRow(t *testing.T) {
	if got := mustMap(t, scoped(nil, true)); !got.AnnualOnly {
		t.Fatalf("AnnualOnly = false, want true")
	}
}

// TestMapCode_MultiplePlansKeepPublishedOrder guards the whole scope, not just
// its first element.
func TestMapCode_MultiplePlansKeepPublishedOrder(t *testing.T) {
	got := mustMap(t, scoped([]string{"pro", "studio"}, false))

	if len(got.AllowedPlans) != 2 || got.AllowedPlans[0] != "pro" || got.AllowedPlans[1] != "studio" {
		t.Fatalf("AllowedPlans = %v, want [pro studio]", got.AllowedPlans)
	}
}

// TestMapCode_UnscopedCodeMapsToNilNotEmpty is decision 4 of the plan, and the
// reason it is a test rather than a comment: NULL and '{}' are the SAME fact to
// the redeemer (validator.go guards on len() > 0), so only one of the two
// spellings may ever be written. An empty pq.StringArray stores as '{}', which
// reads as "scoped" to anything inspecting the column, and require.Empty would
// accept it just as happily as nil.
func TestMapCode_UnscopedCodeMapsToNilNotEmpty(t *testing.T) {
	for name, in := range map[string][]string{
		"absent": nil,
		// The console refuses to publish this, so it should never arrive —
		// but if it ever does it means "every plan", exactly as nil does.
		"empty": {},
	} {
		t.Run(name, func(t *testing.T) {
			got := mustMap(t, scoped(in, false))
			if got.AllowedPlans != nil {
				t.Fatalf("AllowedPlans = %#v, want nil so the column stores NULL and never '{}'", got.AllowedPlans)
			}
		})
	}
}

// TestMapCode_UnknownPlanIsRejected is decision 3. Storing an unrecognised
// plan would scope the code to something nothing can ever be on: dead at
// redemption, and reading as correctly configured to anyone looking.
func TestMapCode_UnknownPlanIsRejected(t *testing.T) {
	cases := map[string][]string{
		// plangate's plan, priced nowhere; the console does not offer it.
		"trial":                {"trial"},
		"never existed":        {"platinum"},
		"one bad among good":   {"pro", "platinum"},
		"empty string element": {""},
	}
	for name, plans := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := MapCode(scoped(plans, false), now)
			if err == nil {
				t.Fatalf("MapCode(%v): expected a rejection, got none", plans)
			}
			if r := ReasonOf(err); r != ReasonUnknownPlan {
				t.Fatalf("reason = %q, want %q", r, ReasonUnknownPlan)
			}
		})
	}
}

// TestMapCode_UnknownPlanRejectsRatherThanDropping is the failure mode a
// per-plan filter would have: dropping "platinum" from {pro, platinum} still
// leaves a plausible row, so only checking that SOMETHING was rejected would
// pass against it. Nothing may be written for this code at all.
func TestMapCode_UnknownPlanRejectsRatherThanDropping(t *testing.T) {
	got, err := MapCode(scoped([]string{"pro", "platinum"}, false), now)
	if err == nil {
		t.Fatalf("expected a rejection")
	}
	if got.Code != "" || got.AllowedPlans != nil {
		t.Fatalf("a rejected definition must yield the zero row, got %+v", got)
	}
}

// TestMapCode_PlanNamesAreNormalised keeps one spelling in the column. The
// redeemer compares with strings.EqualFold, so this changes no redemption
// outcome — it only stops two publications of the same scope storing
// differently.
func TestMapCode_PlanNamesAreNormalised(t *testing.T) {
	got := mustMap(t, scoped([]string{" PRO ", "Studio", "pro"}, false))

	if len(got.AllowedPlans) != 2 || got.AllowedPlans[0] != "pro" || got.AllowedPlans[1] != "studio" {
		t.Fatalf("AllowedPlans = %v, want [pro studio] (trimmed, lower-cased, deduplicated)", got.AllowedPlans)
	}
}

// TestKnownPlans_IsTheConsoleVocabulary pins the derived set itself. It is
// built from the price catalog, so this fails if a plan is added to pricing
// without the console contract being reconsidered — which is the moment to
// look, not later when a scope is silently rejected in production.
func TestKnownPlans_IsTheConsoleVocabulary(t *testing.T) {
	want := []string{"starter", "studio", "pro"}
	if len(knownPlans) != len(want) {
		t.Fatalf("knownPlans = %v, want exactly %v", knownPlans, want)
	}
	for _, p := range want {
		if _, ok := knownPlans[p]; !ok {
			t.Fatalf("knownPlans is missing %q; it holds %v", p, knownPlans)
		}
	}
	if _, ok := knownPlans["trial"]; ok {
		t.Fatalf("trial is a plangate plan with no price and must not be a promo scope")
	}
}

// TestSync_UnknownPlanIsSkippedWithAReasonAndCostsOnlyItsOwnCode is decision 3
// at the sync level: the rejection has to arrive as a COUNTED skip with a
// named reason, not as a silent drop and not as a failed batch. The sibling
// code in the same publication must still be written.
func TestSync_UnknownPlanIsSkippedWithAReasonAndCostsOnlyItsOwnCode(t *testing.T) {
	f := &fakeFetcher{cat: Catalog{RevisionID: "rev-scope", Codes: []Code{
		scoped([]string{"platinum"}, false),
		{Code: "EXTRA14DAYS", TrialExtensionDays: ptr(14)},
	}}}
	s := &fakeStore{}

	res, err := newSyncer(f, s).Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if res.Ingested != 1 || res.Skipped != 1 {
		t.Fatalf("ingested=%d skipped=%d, want 1 and 1", res.Ingested, res.Skipped)
	}
	if got := res.SkippedByReason[ReasonUnknownPlan]; got != 1 {
		t.Fatalf("SkippedByReason[%s] = %d, want 1 (breakdown: %v)",
			ReasonUnknownPlan, got, res.SkippedByReason)
	}
	if len(s.upserted) != 1 || len(s.upserted[0]) != 1 || s.upserted[0][0].Code != "EXTRA14DAYS" {
		t.Fatalf("upserted = %+v, want only EXTRA14DAYS", s.upserted)
	}

	// The rejected code is still KEPT, not expired. It is present in the
	// catalog and merely unreadable by us; expiring it would turn a scope we
	// could not parse into a withdrawn campaign for a merchant.
	if len(s.keptCalls) != 1 || len(s.keptCalls[0]) != 2 {
		t.Fatalf("kept = %v, want both published codes", s.keptCalls)
	}
}
