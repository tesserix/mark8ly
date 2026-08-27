package platformadmin

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/billing/migration"
	"github.com/mark8ly/marketplace-api/internal/inbox"
)

// ErrOperatorNotAddressable is returned when the platform operator id cannot
// be used as a domain reviewer id.
//
// migration_fast_path_reviews.reviewer_id is a uuid, and the platform surface
// identifies operators with the free-text X-Platform-Operator header. Rather
// than synthesise a UUID from that string, this refuses: a fabricated id in a
// column named reviewer_id reads as a real user to everyone who queries it
// later, and there is no FK to catch it. Attribution still lands in the audit
// row, which carries the readable operator in actor_operator_id.
var ErrOperatorNotAddressable = errors.New("platformadmin: operator id is not a uuid")

// MigrationFastPathReviewer is the slice of migration.Repository this executor
// needs. Both methods return the post-update row so the audit event can be
// attributed to the review's own tenant and store rather than to whatever the
// caller claimed.
type MigrationFastPathReviewer interface {
	Approve(ctx context.Context, id, reviewerID uuid.UUID, notes string) (*migration.Review, error)
	Reject(ctx context.Context, id, reviewerID uuid.UUID, notes string) (*migration.Review, error)
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
	reviewerID, err := uuid.Parse(operatorID)
	if err != nil {
		return InboxActionResult{}, ErrOperatorNotAddressable
	}

	var review *migration.Review
	switch actionID {
	case "approve":
		review, err = e.repo.Approve(ctx, reviewID, reviewerID, notes)
	case "reject":
		review, err = e.repo.Reject(ctx, reviewID, reviewerID, notes)
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
