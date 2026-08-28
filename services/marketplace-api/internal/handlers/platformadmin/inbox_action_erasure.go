package platformadmin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/customererasure"
	"github.com/mark8ly/marketplace-api/internal/inbox"
)

// CustomerEraser is the slice of *customererasure.Executor this action needs,
// declared here as a local interface for the same reason every other
// dependency in this package is: the handler stays testable without a
// database, and this package does not depend on the concrete executor.
//
// Lookup is separate from Process because a Receipt deliberately carries no
// tenant or store — it is evidence about data, not about ownership — and the
// audit row needs a tenant to be attributable at all (#310).
type CustomerEraser interface {
	Process(ctx context.Context, requestID uuid.UUID) (customererasure.Receipt, error)
	Reject(ctx context.Context, requestID uuid.UUID, notes string) (customererasure.Request, error)
	Lookup(ctx context.Context, requestID uuid.UUID) (customererasure.Request, error)
}

// ErasureExecutor executes process/reject for the erasure_request inbox kind
// (#259).
//
// This is the surface that turns a GDPR art.17 request into IRREVERSIBLE
// destruction of a person's data, so two things are deliberate and neither is
// defensive padding:
//
//   - An unknown actionID is an explicit error, never a fall-through. The
//     handler already checks the item's declared actions, so this is
//     unreachable today; it exists so that an action added to
//     inbox.ErasureProvider's declaration but not implemented here fails
//     loudly rather than landing in whatever branch happens to be last.
//   - A missing operator id is refused. An erasure that cannot be attributed
//     to anyone must not run: the audit row would record an anonymous
//     destruction of customer data, and there is nothing to undo it with.
type ErasureExecutor struct{ eraser CustomerEraser }

// NewErasureExecutor constructs the executor.
func NewErasureExecutor(eraser CustomerEraser) *ErasureExecutor {
	return &ErasureExecutor{eraser: eraser}
}

func (e *ErasureExecutor) Kind() string { return inbox.KindErasureRequest }

func (e *ErasureExecutor) Execute(
	ctx context.Context, item inbox.Item, actionID, operatorID, notes string,
) (InboxActionResult, error) {
	requestID, err := uuid.Parse(item.ID)
	if err != nil {
		return InboxActionResult{}, fmt.Errorf("erasure: item id is not a uuid: %w", err)
	}
	if strings.TrimSpace(operatorID) == "" {
		return InboxActionResult{}, ErrMissingOperator
	}

	switch actionID {
	case "process":
		return e.process(ctx, requestID, item.ID)
	case "reject":
		return e.reject(ctx, requestID, item.ID, notes)
	default:
		return InboxActionResult{}, fmt.Errorf("erasure: unsupported action %q", actionID)
	}
}

func (e *ErasureExecutor) process(ctx context.Context, requestID uuid.UUID, itemID string) (InboxActionResult, error) {
	if _, err := e.eraser.Process(ctx, requestID); err != nil {
		return InboxActionResult{}, translateErasureErr(err, itemID)
	}
	// Read AFTER the erasure, not before: the row is still there (its own
	// status is the receipt) and reading it afterwards means the attribution
	// describes a request that actually completed, rather than one that was
	// merely about to be attempted.
	req, err := e.eraser.Lookup(ctx, requestID)
	if err != nil {
		return InboxActionResult{}, translateErasureErr(err, itemID)
	}
	return erasureResult(req), nil
}

func (e *ErasureExecutor) reject(ctx context.Context, requestID uuid.UUID, itemID, notes string) (InboxActionResult, error) {
	req, err := e.eraser.Reject(ctx, requestID, notes)
	if err != nil {
		return InboxActionResult{}, translateErasureErr(err, itemID)
	}
	return erasureResult(req), nil
}

func erasureResult(req customererasure.Request) InboxActionResult {
	storeID := req.StoreID
	return InboxActionResult{TenantID: req.TenantID, StoreID: &storeID, Status: req.Status}
}

// translateErasureErr maps a claim that another worker or operator already
// took into the inbox vocabulary, so the handler answers 409 "already
// actioned" rather than 500 — exactly as the migration executor does for a
// second decision. Two operators clicking the same queue row is the common
// race, and 409 is the answer that tells them what happened.
func translateErasureErr(err error, itemID string) error {
	if errors.Is(err, customererasure.ErrAlreadyClaimed) || errors.Is(err, customererasure.ErrRequestNotFound) {
		return fmt.Errorf("%w: %s", inbox.ErrItemNotFound, itemID)
	}
	// Everything else is surfaced as-is. The erasure package's own errors are
	// already free of the subject's personal data (see StepError), which
	// matters because the handler logs this.
	return err
}
