package arbitrage

import (
	"context"

	"github.com/google/uuid"
)

// PIIAccessEvent describes one read of the arbitrage audit table.
// The audit-service is the system-of-record; we just emit so compliance has
// a log trail (§18.8: "every read logged").
type PIIAccessEvent struct {
	Actor     uuid.UUID // billing-ops user; uuid.Nil for system jobs
	StoreID   uuid.UUID
	TenantID  uuid.UUID
	Operation string // e.g. "arbitrage_audit_read_admin_subscription"
	Note      string // optional free text for error context
}

// PIILogger is the injection point; production wires to the audit-service
// client; tests inject NopPIILogger or a spy.
type PIILogger interface {
	LogPIIAccess(ctx context.Context, evt PIIAccessEvent)
}

// NopPIILogger silently discards PII access events. Use in tests where PII
// logging is not under assertion.
type NopPIILogger struct{}

func (NopPIILogger) LogPIIAccess(context.Context, PIIAccessEvent) {}

// AuditEmittable is a minimal audit event struct compatible with the
// internal/audit.Emitter shape (avoids pulling the whole package in here).
type AuditEmittable struct {
	Kind     string
	Severity string
	Metadata map[string]string
	TenantID uuid.UUID
	StoreID  uuid.UUID
	ActorID  uuid.UUID
}

// AuditEmitter is the minimal slice of internal/audit.Emitter used here.
type AuditEmitter interface {
	Emit(ctx context.Context, evt AuditEmittable)
}

// AuditServicePIILogger wraps the audit-service emitter to provide PII access
// logging with a consistent event kind.
type AuditServicePIILogger struct {
	Emitter AuditEmitter
	Source  string // "marketplace-api/arbitrage"
}

func (l AuditServicePIILogger) LogPIIAccess(ctx context.Context, evt PIIAccessEvent) {
	md := map[string]string{
		"operation": evt.Operation,
		"source":    l.Source,
	}
	if evt.Note != "" {
		md["note"] = evt.Note
	}
	l.Emitter.Emit(ctx, AuditEmittable{
		Kind:     "arbitrage_pii_access",
		Severity: "info",
		Metadata: md,
		TenantID: evt.TenantID,
		StoreID:  evt.StoreID,
		ActorID:  evt.Actor,
	})
}
