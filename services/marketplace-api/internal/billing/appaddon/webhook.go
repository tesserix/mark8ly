package appaddon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// InvoicePaidKindAddOn is the metadata.kind value we stamp on the
// one-off invoice in handler.go. HandleInvoicePaidForAppAddOn filters
// on this so the generic invoice.paid handler stays untouched for
// regular subscription invoices.
const InvoicePaidKindAddOn = "white_label_app_add_on"

// invoicePaidEnvelope parses only the metadata we actually need. The
// dispatcher passes the full Stripe event JSON through; we don't
// define the whole Stripe schema, just the two labels we need to
// look at.
type invoicePaidEnvelope struct {
	Data struct {
		Object struct {
			ID       string            `json:"id"`
			Metadata map[string]string `json:"metadata"`
		} `json:"object"`
	} `json:"data"`
}

// HandleInvoicePaidForAppAddOn is the sub-handler the dispatcher calls
// after the generic invoice.paid handler. When Stripe's invoice
// metadata carries kind=white_label_app_add_on, we flip the flag on
// store_subscriptions; otherwise no-op.
//
// Idempotent: the UPDATE's WHERE has_white_label_app_add_on = FALSE
// clause CAS-guards against replays — a second run where the flag is
// already true becomes a zero-row update.
//
// The caller holds pg_advisory_xact_lock on the store already
// (dispatcher wraps every handler in subscription.WithAdvisoryLock),
// so no additional locking is needed here.
func HandleInvoicePaidForAppAddOn(ctx context.Context, tx *gorm.DB, raw []byte) error {
	var e invoicePaidEnvelope
	if err := json.Unmarshal(raw, &e); err != nil {
		return fmt.Errorf("appaddon webhook: unmarshal: %w", err)
	}
	obj := e.Data.Object
	if obj.Metadata["kind"] != InvoicePaidKindAddOn {
		return nil // not our invoice
	}

	storeIDStr := obj.Metadata["store_id"]
	if storeIDStr == "" {
		return errors.New("appaddon webhook: missing store_id metadata")
	}
	storeID, err := uuid.Parse(storeIDStr)
	if err != nil {
		return fmt.Errorf("appaddon webhook: invalid store_id: %w", err)
	}

	// CAS flip — only flip if currently false AND plan is Pro. The
	// plan check is defensive: if a Pro store downgraded between
	// purchase and payment, we skip the flag (and the separate
	// dunning path will invalidate the in-flight add-on invoice).
	res := tx.WithContext(ctx).Exec(`
		UPDATE store_subscriptions
		SET has_white_label_app_add_on = TRUE,
		    updated_at                 = now()
		WHERE store_id              = ?
		  AND plan                  = ?
		  AND has_white_label_app_add_on = FALSE`,
		storeID, string(subscription.PlanPro),
	)
	if res.Error != nil {
		return fmt.Errorf("appaddon webhook: flip flag: %w", res.Error)
	}
	// RowsAffected == 0 is fine — either a replay or the store
	// downgraded. Not an error state.
	return nil
}
