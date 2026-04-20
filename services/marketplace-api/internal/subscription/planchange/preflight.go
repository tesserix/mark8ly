package planchange

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/statemachine"
)

// PreflightDecision classifies the outcome of a preflight check so the UI
// can render the appropriate confirmation dialog or error state.
type PreflightDecision string

const (
	// DecisionAllowUpgradeNow — immediate upgrade; merchant can proceed.
	DecisionAllowUpgradeNow PreflightDecision = "allow_upgrade_now"

	// DecisionAllowDowngradeAtPeriodEnd — downgrade will be scheduled for
	// period-end; merchant can proceed with the understanding it is deferred.
	DecisionAllowDowngradeAtPeriodEnd PreflightDecision = "allow_downgrade_at_period_end"

	// DecisionAllowPeriodSwitch — same plan, different billing period; immediate.
	DecisionAllowPeriodSwitch PreflightDecision = "allow_period_switch"

	// DecisionNoOp — target is identical to current plan + period.
	DecisionNoOp PreflightDecision = "no_op"

	// DecisionBlockReadOnly — subscription is in a terminal / read-only status.
	DecisionBlockReadOnly PreflightDecision = "block_read_only"

	// DecisionBlockCurrency — requested currency differs from the locked
	// billing currency on the subscription.
	DecisionBlockCurrency PreflightDecision = "block_currency"

	// DecisionBlockStoreCount — downgrade is blocked because the merchant has
	// more stores than the target plan allows.
	DecisionBlockStoreCount PreflightDecision = "block_store_count"
)

// StoreInfo carries per-store details surfaced in the preflight report when a
// downgrade would be blocked by store-count quota. The UI uses this to tell
// the merchant exactly which stores need to be archived.
type StoreInfo struct {
	StoreID        uuid.UUID `json:"store_id"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	Status         string    `json:"status"`
	InFlightOrders int       `json:"in_flight_orders"`
	// IsSoftDeleted is always false: the local stores projection does not carry
	// deleted_at (that column lives in platform-api). Included for forward-
	// compatibility when the projection is enriched.
	IsSoftDeleted bool `json:"is_soft_deleted"`
}

// LimitDeltaKind discriminates the shape of a LimitDelta so the UI can
// render "Unlimited" vs "Contact Sales" vs a concrete numeric delta
// without guessing from sentinel ints.
type LimitDeltaKind string

const (
	// LimitDeltaNumeric — Value is the concrete target − current.
	LimitDeltaNumeric LimitDeltaKind = "numeric"
	// LimitDeltaUnlimited — one or both sides is Unlimited; the UI
	// should render "Unlimited" rather than a numeric difference.
	LimitDeltaUnlimited LimitDeltaKind = "unlimited"
	// LimitDeltaNegotiated — at least one side is Negotiated; the UI
	// should render "Contact Sales" (enterprise tier).
	LimitDeltaNegotiated LimitDeltaKind = "negotiated"
)

// LimitDelta carries both a numeric delta and a discriminator. The
// legacy *Delta int fields on PlanDiff are preserved for backward
// compat; new consumers should read the *DeltaTagged fields instead.
type LimitDelta struct {
	Kind  LimitDeltaKind `json:"kind"`
	Value int            `json:"value,omitempty"` // only meaningful when Kind=numeric
}

// PlanDiff describes the feature-limit changes between the current plan and
// the target plan. Positive numeric deltas are gains; negative are reductions.
// A legacy numeric delta of plangate.Unlimited (-1) means one or both sides
// is uncapped or negotiated — see the *DeltaTagged fields for a richer
// representation that distinguishes the two.
type PlanDiff struct {
	// StoresDelta is the change in the stores limit (target − current).
	StoresDelta int `json:"stores_delta"`
	// ImagesPerProductDelta is the change in the images-per-product cap.
	// Set to plangate.Unlimited (-1) if either side is Unlimited or Negotiated.
	ImagesPerProductDelta int `json:"images_per_product_delta"`
	// CampaignEmailsDelta is the change in campaign emails per month.
	// Set to plangate.Unlimited (-1) if either side is Unlimited or Negotiated.
	CampaignEmailsDelta int `json:"campaign_emails_delta"`

	// Tagged deltas — preferred. Let the UI render "Unlimited" vs
	// "Contact Sales" correctly without inspecting sentinel ints.
	StoresDeltaTagged           LimitDelta `json:"stores_delta_tagged"`
	ImagesPerProductDeltaTagged LimitDelta `json:"images_per_product_delta_tagged"`
	CampaignEmailsDeltaTagged   LimitDelta `json:"campaign_emails_delta_tagged"`
}

// PreflightReport is the full read-only summary returned to the UI before the
// merchant commits to a plan change. All fields are safe to serialise as JSON.
type PreflightReport struct {
	Decision PreflightDecision `json:"decision"`

	// CurrentPlan / TargetPlan identify the transition for display.
	CurrentPlan   subscription.SubscriptionPlan   `json:"current_plan"`
	CurrentPeriod subscription.SubscriptionPeriod `json:"current_period"`
	TargetPlan    subscription.SubscriptionPlan   `json:"target_plan"`
	TargetPeriod  subscription.SubscriptionPeriod `json:"target_period"`

	// EffectiveAt is when the change will take effect. Zero for blocking decisions.
	EffectiveAt time.Time `json:"effective_at,omitempty"`

	// CurrentPlanDiff describes what the merchant gains or loses.
	CurrentPlanDiff PlanDiff `json:"current_plan_diff"`

	// Store-count details — only populated for block_store_count decisions.
	StoreCount       int         `json:"store_count,omitempty"`
	TargetStoreLimit int         `json:"target_store_limit,omitempty"`
	Stores           []StoreInfo `json:"stores,omitempty"`
}

// Preflight runs a read-only check and returns a PreflightReport that the UI
// surfaces before the merchant commits to a plan change. It never mutates any
// database row. The returned report is a value type so callers cannot
// accidentally alias its fields.
//
// The method is defined on *Orchestrator so it shares Deps (DB, repos, clock)
// without requiring a separate constructor.
func (o *Orchestrator) Preflight(ctx context.Context, in Input) (PreflightReport, error) {
	if in.Now.IsZero() {
		in.Now = o.deps.Clock.Now()
	}

	sub, err := o.deps.SubscriptionRepo.GetByStoreID(ctx, o.deps.DB, in.TenantID, in.StoreID)
	if err != nil {
		return PreflightReport{}, fmt.Errorf("planchange: preflight load subscription: %w", err)
	}

	base := PreflightReport{
		CurrentPlan:     sub.Plan,
		CurrentPeriod:   sub.SubscriptionPeriod,
		TargetPlan:      in.TargetPlan,
		TargetPeriod:    in.TargetPeriod,
		CurrentPlanDiff: buildPlanDiff(sub.Plan, in.TargetPlan),
	}

	// Block: subscription is in a read-only / terminal status.
	if statemachine.IsReadOnly(sub.Status) {
		base.Decision = DecisionBlockReadOnly
		return base, nil
	}

	// Block: currency mismatch. Mirrors the same check in Execute so the UI
	// sees the block before the merchant attempts to commit.
	if in.RequestedCurrency != "" && sub.BillingCurrency != nil &&
		!strings.EqualFold(in.RequestedCurrency, *sub.BillingCurrency) {
		base.Decision = DecisionBlockCurrency
		return base, nil
	}

	dir := Classify(sub.Plan, sub.SubscriptionPeriod, in.TargetPlan, in.TargetPeriod)

	// No-op: target is identical to current.
	if dir == DirectionNoChange {
		base.Decision = DecisionNoOp
		return base, nil
	}

	// Upgrade or period-switch.
	if dir == DirectionUpgrade {
		if sub.Plan == in.TargetPlan {
			// Same plan rank, period-only change (monthly → annual).
			base.Decision = DecisionAllowPeriodSwitch
		} else {
			base.Decision = DecisionAllowUpgradeNow
		}
		base.EffectiveAt = in.Now
		return base, nil
	}

	// Downgrade path — may be blocked by store-count quota.
	if RequiresStoreCountCheck(sub.Plan, in.TargetPlan) {
		storeLimit := plangate.Limit(in.TargetPlan, plangate.FeatureStores)

		count, err := o.deps.StoreRepo.CountActiveOrSoftDeletedRestorable(ctx, in.TenantID)
		if err != nil {
			return PreflightReport{}, fmt.Errorf("planchange: preflight count stores: %w", err)
		}

		if storeLimit != plangate.Unlimited && count > storeLimit {
			storeRows, err := o.deps.StoreRepo.ListActiveOrSoftDeletedRestorable(ctx, in.TenantID)
			if err != nil {
				return PreflightReport{}, fmt.Errorf("planchange: preflight list stores: %w", err)
			}

			infos := buildStoreInfos(ctx, o.deps.StoreRepo, storeRows)

			base.Decision = DecisionBlockStoreCount
			base.StoreCount = count
			base.TargetStoreLimit = storeLimit
			base.Stores = infos
			return base, nil
		}
	}

	// Downgrade is allowed — deferred to period-end.
	var periodEnd = in.Now
	if sub.CurrentPeriodEnd != nil {
		periodEnd = *sub.CurrentPeriodEnd
	}
	effective, _ := EffectiveAt(DirectionDowngrade, in.Now, periodEnd)

	base.Decision = DecisionAllowDowngradeAtPeriodEnd
	base.EffectiveAt = effective
	return base, nil
}

// buildPlanDiff computes the feature-limit deltas between fromPlan and toPlan.
// Populates both the legacy numeric fields and the tagged fields so new
// consumers can distinguish "Unlimited" from "Contact Sales" without
// inspecting sentinel ints.
func buildPlanDiff(from, to subscription.SubscriptionPlan) PlanDiff {
	storesFrom, storesTo := plangate.Limit(from, plangate.FeatureStores), plangate.Limit(to, plangate.FeatureStores)
	imagesFrom, imagesTo := plangate.Limit(from, plangate.FeatureImagesPerProduct), plangate.Limit(to, plangate.FeatureImagesPerProduct)
	emailsFrom, emailsTo := plangate.Limit(from, plangate.FeatureCampaignEmailsPerMonth), plangate.Limit(to, plangate.FeatureCampaignEmailsPerMonth)
	return PlanDiff{
		StoresDelta:                 limitDelta(storesFrom, storesTo),
		ImagesPerProductDelta:       limitDelta(imagesFrom, imagesTo),
		CampaignEmailsDelta:         limitDelta(emailsFrom, emailsTo),
		StoresDeltaTagged:           taggedDelta(storesFrom, storesTo),
		ImagesPerProductDeltaTagged: taggedDelta(imagesFrom, imagesTo),
		CampaignEmailsDeltaTagged:   taggedDelta(emailsFrom, emailsTo),
	}
}

// limitDelta returns (to - from). When either value is non-positive (sentinel:
// Unlimited = -1, Negotiated = -2, Disabled = 0), the result is
// plangate.Unlimited (-1) to signal the UI should not render a raw number.
// Preserved for backward compatibility with the numeric *Delta fields; new
// callers should use taggedDelta.
func limitDelta(from, to int) int {
	if from <= 0 || to <= 0 {
		return plangate.Unlimited
	}
	return to - from
}

// taggedDelta returns a LimitDelta whose Kind distinguishes Unlimited from
// Negotiated. Negotiated trumps Unlimited (enterprise > uncapped) so that
// a numeric → negotiated transition is rendered as "Contact Sales".
// Disabled is collapsed into LimitDeltaUnlimited today because no current
// surface transitions across a Disabled numeric feature — revisit when one
// does.
func taggedDelta(from, to int) LimitDelta {
	if from == plangate.Negotiated || to == plangate.Negotiated {
		return LimitDelta{Kind: LimitDeltaNegotiated}
	}
	if from <= 0 || to <= 0 {
		return LimitDelta{Kind: LimitDeltaUnlimited}
	}
	return LimitDelta{Kind: LimitDeltaNumeric, Value: to - from}
}

// buildStoreInfos constructs a []StoreInfo from stores.Store rows, enriching
// each entry with its in-flight order count via InFlightOrderCount. Errors
// from InFlightOrderCount are non-fatal per row — the count stays zero and
// processing continues, so a transient DB hiccup does not abort the report.
func buildStoreInfos(ctx context.Context, repo interface {
	InFlightOrderCount(ctx context.Context, storeID uuid.UUID) (int, error)
}, rows []stores.Store) []StoreInfo {
	out := make([]StoreInfo, 0, len(rows))
	for _, s := range rows {
		id, err := uuid.Parse(s.ID)
		if err != nil {
			// Malformed UUID in projection row — use Nil and carry on.
			id = uuid.Nil
		}
		n, _ := repo.InFlightOrderCount(ctx, id) // non-fatal; see doc above
		out = append(out, StoreInfo{
			StoreID:        id,
			Name:           s.Name,
			Slug:           s.Slug,
			Status:         s.Status,
			InFlightOrders: n,
			IsSoftDeleted:  false, // projection has no deleted_at; see StoreInfo doc
		})
	}
	return out
}
