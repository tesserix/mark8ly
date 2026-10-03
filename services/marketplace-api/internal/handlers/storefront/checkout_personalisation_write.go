package storefront

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/personalisationupload"
)

// persistPersonalisation snapshots the buyer's answers onto the order and
// claims the uploads they used.
//
// MUST run inside the order-create transaction. Both halves are
// load-bearing and neither is safe alone:
//
//   - without the snapshot, the order charges a surcharge and records
//     nothing about what was bought;
//   - without the claim, personalisation-upload-sweep-cron destroys the
//     artwork of a paid order 72 hours later, because the sweeper's only
//     question is "did anyone claim this".
//
// Running them in separate transactions would make a crash between them
// produce exactly one of those two outcomes, so they go together or not
// at all.
func persistPersonalisation(
	ctx context.Context,
	tx *gorm.DB,
	orderItems []orderItemRef,
	resolved [][]resolvedPersonalisation,
) error {
	var claimedUploadIDs []string

	for i, answers := range resolved {
		if len(answers) == 0 {
			continue
		}
		if i >= len(orderItems) {
			// The two slices are positional by construction; a mismatch
			// means the caller changed one without the other, and
			// guessing which line an answer belonged to would attach a
			// buyer's photograph to the wrong product.
			return fmt.Errorf("storefront: personalisation/line mismatch at %d", i)
		}
		itemID := orderItems[i].ID

		for pos := range answers {
			a := &answers[pos]
			row := map[string]any{
				"order_item_id":        itemID,
				"field_key":            a.FieldKey,
				"field_label":          a.FieldLabel,
				"kind":                 a.Kind,
				"text_value":           a.TextValue,
				"price_delta":          a.PriceDelta,
				"storage_key_original": a.StorageKeyOriginal,
				"storage_key":          a.StorageKey,
				"content_type":         a.ContentType,
				"size_bytes":           a.SizeBytes,
				"original_filename":    a.OriginalFilename,
				"position":             a.Position,
			}
			if len(a.Crop) > 0 {
				row["crop"] = a.Crop
			}
			if err := tx.WithContext(ctx).
				Table("order_item_personalisations").
				Create(row).Error; err != nil {
				return fmt.Errorf("storefront: snapshot personalisation: %w", err)
			}
			if a.UploadID != "" {
				claimedUploadIDs = append(claimedUploadIDs, a.UploadID)
			}
		}
	}

	if len(claimedUploadIDs) == 0 {
		return nil
	}

	// Claim every upload this order used, in one statement. `claimed` is
	// what makes the sweeper leave it alone — see the partial index on
	// personalisation_uploads, which excludes claimed rows entirely.
	res := tx.WithContext(ctx).
		Model(&personalisationupload.Upload{}).
		Where("id IN ?", claimedUploadIDs).
		Updates(map[string]any{
			"state":      personalisationupload.StateClaimed,
			"updated_at": gorm.Expr("now()"),
		})
	if res.Error != nil {
		return fmt.Errorf("storefront: claim uploads: %w", res.Error)
	}
	if int(res.RowsAffected) != len(claimedUploadIDs) {
		// A row vanished between validation and here — swept, deleted by
		// the buyer in another tab, or never ours. Unwinding the order is
		// the only safe answer: the alternative is an order whose artwork
		// is already gone or belongs to someone else.
		return fmt.Errorf(
			"storefront: expected to claim %d uploads, claimed %d",
			len(claimedUploadIDs), res.RowsAffected)
	}
	return nil
}

// orderItemRef is the slice of an order item this file needs. A named
// type rather than order.OrderItem so the dependency stays one field
// wide and the positional contract is obvious at the call site.
type orderItemRef struct{ ID string }
