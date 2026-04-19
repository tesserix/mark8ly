package audit

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// PromoApplied carries parameters for the subscription.promo_applied audit event.
type PromoApplied struct {
	TenantID     uuid.UUID
	StoreID      uuid.UUID
	Code         string
	Actor        string
	// RejectReason is the internal reason (never sent to the HTTP client).
	RejectReason string
	// Accepted indicates whether the promo was successfully applied.
	Accepted     bool
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
