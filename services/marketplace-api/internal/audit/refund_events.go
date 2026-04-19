package audit

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RefundIssued carries parameters for the subscription.refund_issued audit event.
type RefundIssued struct {
	TenantID uuid.UUID
	StoreID  uuid.UUID
	// StripeChargeID is the charge that was (or would have been) refunded.
	StripeChargeID string
	Actor          string
	// Accepted indicates whether the refund was successfully issued.
	Accepted bool
}

// EmitRefundIssued emits a subscription.refund_issued audit event.
// When Accepted is false, severity is Warning — a rejected refund request
// is a billing signal that ops should be able to query (§8, §23.1).
func (e *Emitter) EmitRefundIssued(c *gin.Context, r RefundIssued) {
	severity := SeverityInfo
	if !r.Accepted {
		severity = SeverityWarning
	}

	e.Emit(c, Event{
		Action:       "subscription.refund_issued",
		ResourceType: "subscription",
		ResourceID:   r.StoreID.String(),
		Severity:     severity,
		Metadata: map[string]any{
			"stripe_charge_id": r.StripeChargeID,
			"actor":            r.Actor,
			"accepted":         r.Accepted,
		},
		TenantID:       r.TenantID,
		StoreID:        r.StoreID,
		ForceActorType: classifyActor(r.Actor),
	})
}

// BillingArchived carries parameters for the billing.archive_created and
// billing.archive_expired audit events.
type BillingArchived struct {
	TenantID  uuid.UUID
	StoreID   uuid.UUID
	ArchiveID uuid.UUID
	// Op is "created" or "expired".
	Op    string
	Actor string
}

// EmitBillingArchived emits a billing.archive_created or billing.archive_expired
// audit event depending on Op.
func (e *Emitter) EmitBillingArchived(c *gin.Context, b BillingArchived) {
	action := "billing.archive_" + b.Op

	e.Emit(c, Event{
		Action:       action,
		ResourceType: "billing_archive",
		ResourceID:   b.ArchiveID.String(),
		Severity:     SeverityInfo,
		Metadata: map[string]any{
			"archive_id": b.ArchiveID.String(),
			"op":         b.Op,
			"actor":      b.Actor,
		},
		TenantID:       b.TenantID,
		StoreID:        b.StoreID,
		ForceActorType: classifyActor(b.Actor),
	})
}
