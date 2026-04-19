package audit

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CredentialAccess records a single read/write/delete on a merchant-scoped
// Secret Manager secret. Used by the white-label app add-on's credential
// choke-point (internal/billing/appcreds) — see spec §18.9.
//
// Every access through appcreds.Service emits one CredentialAccess event.
// The pair (actor, operation) gives legal + security the full audit trail
// required for compliance reviews without leaking the secret payload
// itself (the payload never appears on the event).
type CredentialAccess struct {
	TenantID       uuid.UUID
	StoreID        uuid.UUID
	CredentialType string // matches appcreds.CredType (closed enum of 4 values)
	Operation      string // "read" | "write" | "delete"
	Actor          string // "user:<uuid>" | "system:cron:lifecycle" | "system:ci"
}

// EmitCredentialAccess is the canonical hook for white-label credential
// I/O audit. Mirrors the EmitStateTransition / EmitAPIKeyEvent shape so
// dashboards can union the streams.
//
// Severity escalates for delete + system actors; a system-initiated
// delete is how the day-90 purge path surfaces, and ops dashboards
// watch for it to confirm teardown completed.
func (e *Emitter) EmitCredentialAccess(c *gin.Context, ev CredentialAccess) {
	md := map[string]any{
		"credential_type": ev.CredentialType,
		"operation":       ev.Operation,
		"actor":           ev.Actor,
	}

	severity := SeverityInfo
	if ev.Operation == "delete" {
		// Deletes are rare + load-bearing for teardown — bump so alert
		// dashboards catch misfires.
		severity = SeverityWarning
	}

	e.Emit(c, Event{
		Action:         "app_credential." + ev.Operation,
		ResourceType:   "app_credential",
		ResourceID:     ev.CredentialType,
		Severity:       severity,
		Metadata:       md,
		TenantID:       ev.TenantID,
		StoreID:        ev.StoreID,
		ForceActorType: classifyActor(ev.Actor),
	})
}
