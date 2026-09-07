package audit

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PromoApplied carries parameters for the subscription.promo_applied audit event.
type PromoApplied struct {
	TenantID uuid.UUID
	StoreID  uuid.UUID
	Code     string
	Actor    string
	// RejectReason is the internal reason (never sent to the HTTP client).
	RejectReason string
	// Accepted indicates whether the promo was successfully applied.
	Accepted bool
	// TrialExtensionDays and TrialEndsAt record a trial extension this code
	// granted (#620), and are omitted from the metadata when it granted
	// none.
	//
	// Recorded because the code alone does not say what changed: a promo can
	// move a merchant's billing date, which is the same consequential write
	// an operator extension emits its own audit row for. Without these, the
	// only trace of a moved trial end is a code string whose definition
	// lives in another system and can be edited after the fact.
	TrialExtensionDays int
	TrialEndsAt        time.Time
}

// EmitPromoApplied emits a subscription.promo_applied audit event.
// When Accepted is false, RejectReason is recorded in metadata and
// severity is set to Warning (reject is a business signal, not an error).
func (e *Emitter) EmitPromoApplied(c *gin.Context, p PromoApplied) {
	md := map[string]any{
		"code":     p.Code,
		"actor":    p.Actor,
		"accepted": p.Accepted,
	}
	if p.RejectReason != "" {
		md["reject_reason"] = p.RejectReason
	}
	if p.TrialExtensionDays > 0 {
		md["trial_extension_days"] = p.TrialExtensionDays
	}
	if !p.TrialEndsAt.IsZero() {
		md["trial_ends_at"] = p.TrialEndsAt.UTC().Format(time.RFC3339)
	}

	severity := SeverityInfo
	if !p.Accepted {
		severity = SeverityWarning
	}

	e.Emit(c, Event{
		Action:         "subscription.promo_applied",
		ResourceType:   "subscription",
		ResourceID:     p.StoreID.String(),
		Severity:       severity,
		Metadata:       md,
		TenantID:       p.TenantID,
		StoreID:        p.StoreID,
		ForceActorType: classifyActor(p.Actor),
	})
}

// PromoCancelled carries parameters for the subscription.promo_cancelled audit event.
type PromoCancelled struct {
	TenantID    uuid.UUID
	StoreID     uuid.UUID
	PromoCodeID uuid.UUID
	Actor       string
}

// EmitPromoCancelled emits a subscription.promo_cancelled audit event.
func (e *Emitter) EmitPromoCancelled(c *gin.Context, p PromoCancelled) {
	e.Emit(c, Event{
		Action:       "subscription.promo_cancelled",
		ResourceType: "subscription",
		ResourceID:   p.StoreID.String(),
		Severity:     SeverityInfo,
		Metadata: map[string]any{
			"promo_code_id": p.PromoCodeID.String(),
			"actor":         p.Actor,
		},
		TenantID:       p.TenantID,
		StoreID:        p.StoreID,
		ForceActorType: classifyActor(p.Actor),
	})
}
