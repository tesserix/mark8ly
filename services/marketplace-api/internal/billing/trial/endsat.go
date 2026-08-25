package trial

import (
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// EndsAt returns when this subscription's trial ends.
//
// THIS IS THE ONLY DEFINITION OF TRIAL END. Before #353 the same arithmetic
// was repeated at seven sites, so an operator had nothing to extend: the
// expiry cron, Stripe, the merchant's own screen and the platform console
// each recomputed it from created_at and would have ignored any stored value.
//
// A nil TrialEndsAt — the common case — means the trial has never been
// extended. A non-nil one is authoritative even when it is EARLIER than the
// derived date: shortening is as legitimate as extending, and nothing here
// second-guesses the stored value.
func EndsAt(sub subscription.StoreSubscription) time.Time {
	if sub.TrialEndsAt != nil {
		return sub.TrialEndsAt.UTC()
	}
	return sub.CreatedAt.Add(TrialDays * 24 * time.Hour).UTC()
}

// The three scopes below are EndsAt's SQL counterparts. Each one is a
// two-branch predicate rather than a COALESCE expression, because
// migration 087's (status, created_at) index serves the unextended branch and
// a COALESCE would defeat it; migration 103's partial index serves the
// extended branch, and stays small because extensions are rare.
//
// The branches are duplicated across the three helpers deliberately —
// building them from interpolated comparison operators would read like SQL
// injection and be harder to review. What keeps them honest is
// TestScopesAgreeWithEndsAt, which cross-checks every scope against EndsAt
// over a matrix of extended and unextended rows on each boundary. If a branch
// drifts, that test fails.

// EndedBeforeScope narrows to rows whose effective trial end is strictly
// before at. Used by the expiry cron.
func EndedBeforeScope(db *gorm.DB, at time.Time) *gorm.DB {
	trialLen := time.Duration(TrialDays) * 24 * time.Hour
	return db.Where(
		"(trial_ends_at IS NULL AND created_at < ?) OR (trial_ends_at IS NOT NULL AND trial_ends_at < ?)",
		at.Add(-trialLen), at,
	)
}

// EndsBetweenScope narrows to rows whose effective trial end lies in the
// half-open-left, inclusive-right interval (lo, hi]. Used by the expiring
// queries: half-open left so an already-expired trial is not "expiring",
// inclusive right so one ending exactly at the edge is.
func EndsBetweenScope(db *gorm.DB, lo, hi time.Time) *gorm.DB {
	trialLen := time.Duration(TrialDays) * 24 * time.Hour
	return db.Where(
		"(trial_ends_at IS NULL AND created_at > ? AND created_at <= ?) OR "+
			"(trial_ends_at IS NOT NULL AND trial_ends_at > ? AND trial_ends_at <= ?)",
		lo.Add(-trialLen), hi.Add(-trialLen), lo, hi,
	)
}

// EndsWithinDayScope narrows to rows whose effective trial end falls inside
// the 24 hours beginning at dayStart — [dayStart, dayStart+24h). Used by the
// reminder cron, which fires once per calendar day per offset.
//
// Note the brackets differ from EndsBetweenScope: a day bucket is inclusive
// on the left and exclusive on the right so consecutive days neither overlap
// nor leave a gap.
func EndsWithinDayScope(db *gorm.DB, dayStart time.Time) *gorm.DB {
	trialLen := time.Duration(TrialDays) * 24 * time.Hour
	dayEnd := dayStart.Add(24 * time.Hour)
	return db.Where(
		"(trial_ends_at IS NULL AND created_at >= ? AND created_at < ?) OR "+
			"(trial_ends_at IS NOT NULL AND trial_ends_at >= ? AND trial_ends_at < ?)",
		dayStart.Add(-trialLen), dayEnd.Add(-trialLen), dayStart, dayEnd,
	)
}
