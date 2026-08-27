package platformadmin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/billing/migration"
	"github.com/mark8ly/marketplace-api/internal/inbox"
)

// ErrMissingOperator is returned when no platform operator id is present.
//
// A decision that cannot be attributed to anyone must not be written: the
// audit row and the review row would both record an anonymous approval of a
// merchant's migration.
var ErrMissingOperator = errors.New("platformadmin: no operator id on the request")

// MigrationFastPathReviewer is the slice of migration.Repository this executor
// needs. Both methods return the post-update row so the audit event can be
// attributed to the review's own tenant and store rather than to whatever the
// caller claimed.
//
// The *AsOperator variants are used rather than Approve/Reject because this
// surface identifies operators with a free-text id, not a uuid — every
// operator id production has recorded is an opaque string. They write
// reviewer_operator_id and leave reviewer_id NULL, so nothing fabricates a
// user that does not exist (#281a).
type MigrationFastPathReviewer interface {
	ApproveAsOperator(ctx context.Context, id uuid.UUID, operatorID, notes string) (*migration.Review, error)
	RejectAsOperator(ctx context.Context, id uuid.UUID, operatorID, notes string) (*migration.Review, error)
}

// MigrationFastPathExecutor executes approve/reject for the
// migration_fast_path inbox kind (#281a).
type MigrationFastPathExecutor struct{ repo MigrationFastPathReviewer }

// NewMigrationFastPathExecutor constructs the executor.
func NewMigrationFastPathExecutor(repo MigrationFastPathReviewer) *MigrationFastPathExecutor {
	return &MigrationFastPathExecutor{repo: repo}
}

func (e *MigrationFastPathExecutor) Kind() string { return inbox.KindMigrationFastPath }

func (e *MigrationFastPathExecutor) Execute(
	ctx context.Context, item inbox.Item, actionID, operatorID, notes string,
) (InboxActionResult, error) {
	reviewID, err := uuid.Parse(item.ID)
	if err != nil {
		return InboxActionResult{}, fmt.Errorf("migration fast-path: item id is not a uuid: %w", err)
	}
	if strings.TrimSpace(operatorID) == "" {
		return InboxActionResult{}, ErrMissingOperator
	}

	var review *migration.Review
	switch actionID {
	case "approve":
		review, err = e.repo.ApproveAsOperator(ctx, reviewID, operatorID, notes)
	case "reject":
		review, err = e.repo.RejectAsOperator(ctx, reviewID, operatorID, notes)
	default:
		// Unreachable through the handler, which checks the item's declared
		// actions first. Explicit anyway: a future action added to the
		// provider's declaration but not here must fail loudly, not silently
		// approve.
		return InboxActionResult{}, fmt.Errorf("migration fast-path: unsupported action %q", actionID)
	}

	if err != nil {
		// The repository matches on `status = 'pending'`, so a second
		// decision — or a race with another operator — surfaces as
		// ErrNotFound. Translate it into the inbox vocabulary so the handler
		// answers 409 "already actioned" rather than 500.
		if errors.Is(err, migration.ErrNotFound) {
			return InboxActionResult{}, fmt.Errorf("%w: %s", inbox.ErrItemNotFound, item.ID)
		}
		return InboxActionResult{}, err
	}

	storeID := review.StoreID
	return InboxActionResult{
		TenantID: review.TenantID,
		StoreID:  &storeID,
		Status:   review.Status,
	}, nil
}
