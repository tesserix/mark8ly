package account

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark8ly/platform-api/internal/outbox"
)

// TenantDeletedOutboxKind is the dispatch key for the tenant-teardown
// purge event, enqueued both by teardownTenantTx (merchant self-serve
// delete) and by PurgeTenant (operator-initiated purge, #288).
const TenantDeletedOutboxKind = "tenant.deleted"

// tenantPurger is the subset of marketplaceapi.VendorClient the handler
// needs. Defined locally so *marketplaceapi.VendorClient satisfies it
// without this package importing marketplaceapi, and so tests can supply
// a trivial fake.
type tenantPurger interface {
	PurgeTenant(ctx context.Context, tenantID string, storeIDs []string) error
}

// NewTenantDeletedHandler returns the outbox handler that drives the
// marketplace-side purge for a deleted tenant. It rides the outbox
// (enqueued transactionally by teardownTenantTx) rather than a
// best-effort post-commit call, because a silent failure here would
// leave the tenant's product/order/etc. rows stranded in marketplace-api
// forever with nothing left to re-trigger the purge — the tenant row
// that would have driven a fresh attempt is already gone.
//
// Returning the client error unchanged (never swallowing it) is what
// makes that retry possible: the drainer only marks an event complete
// on a nil return, so any wrapped-but-non-nil error keeps the event
// pending and backs off for another attempt.
func NewTenantDeletedHandler(purger tenantPurger) outbox.Handler {
	return func(ctx context.Context, payload json.RawMessage) error {
		var p tenantDeletedPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return fmt.Errorf("tenant deleted outbox: parse payload: %w", err)
		}
		if p.TenantID == "" {
			return fmt.Errorf("tenant deleted outbox: missing tenant_id")
		}
		if err := purger.PurgeTenant(ctx, p.TenantID, p.StoreIDs); err != nil {
			return fmt.Errorf("tenant deleted outbox: purge tenant %s: %w", p.TenantID, err)
		}
		return nil
	}
}
