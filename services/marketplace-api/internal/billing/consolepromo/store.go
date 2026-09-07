package consolepromo

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mark8ly/marketplace-api/internal/promo"
)

// upsertBatchSize bounds one INSERT. The catalog is small (campaign codes,
// not a product catalog), so this is a guard against a pathological
// publication rather than a tuning knob.
const upsertBatchSize = 200

// gormStore is the production Store.
type gormStore struct{ db *gorm.DB }

// NewStore builds the GORM-backed Store.
func NewStore(db *gorm.DB) Store { return &gormStore{db: db} }

// upsertColumns are the columns a re-sync overwrites from the console.
//
// Everything absent from this list is preserved on an existing row, and there
// are TWO separate reasons for an absence — this is not one rule:
//
//  1. id and created_at belong to the ROW, not to the definition. Rewriting
//     either would re-identify a code that promo_redemptions already points at.
//  2. max_per_email and min_effective_price_per_currency are mark8ly's own
//     abuse controls (§7.3, §7.4) that the console deliberately cannot express
//     (#726). A publication says nothing about them, so it must not clear them.
//
// allowed_plans and annual_only used to sit in group 2 and no longer do
// (#795). The console publishes both, so they are console-owned like every
// other column listed here. Preserving them instead made re-scoping a code the
// ingest had already seen silently never land: the operator narrowed the code
// and the row went on applying to every plan and both periods.
var upsertColumns = []string{
	"stripe_coupon_id",
	"discount_type",
	"discount_value",
	"trial_extension_days",
	"max_duration_months",
	"valid_from",
	"valid_until",
	"max_redemptions",
	"allowed_plans",
	"annual_only",
	"created_by",
	"updated_at",
}

// UpsertCodes writes the mapped rows, keyed on the unique index on code.
//
// valid_until is among the overwritten columns, so a code that was expired
// by a previous sweep and then republished is un-expired by the same
// mechanism that expired it. That is what makes the expiry policy
// recoverable: a console publication is always the last word.
func (s *gormStore) UpsertCodes(ctx context.Context, codes []promo.PromoCode) error {
	if len(codes) == 0 {
		return nil
	}

	// Copy rather than mutate the caller's slice, and assign ids here rather
	// than in the mapper: an id is a storage concern, and generating one in
	// the mapper would make that pure function non-deterministic and its
	// tests unable to compare whole rows. Assigning explicitly also avoids
	// depending on GORM's zero-value handling of the column's
	// DEFAULT gen_random_uuid().
	rows := make([]promo.PromoCode, len(codes))
	copy(rows, codes)
	for i := range rows {
		if rows[i].ID == uuid.Nil {
			rows[i].ID = uuid.New()
		}
	}

	if err := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "code"}},
			DoUpdates: clause.AssignmentColumns(upsertColumns),
		}).
		CreateInBatches(&rows, upsertBatchSize).Error; err != nil {
		return fmt.Errorf("consolepromo: upsert promo codes: %w", err)
	}
	return nil
}

// ExpireCodesNotIn expires console-sourced codes absent from keep.
//
// Three parts of the WHERE clause each carry a decision:
//
//   - created_by = CreatedBy scopes the sweep to rows THIS ingest wrote. A
//     code created by any other path is none of this package's business, and
//     without this an empty catalog would expire the lot.
//   - the valid_until predicate skips rows that are already expired, so a
//     re-run is idempotent and does not keep pushing their expiry forward.
//   - keep is applied only when non-empty. An empty catalog legitimately
//     expires every console-sourced code (no campaigns are running), and
//     `NOT IN ()` is not valid SQL.
func (s *gormStore) ExpireCodesNotIn(ctx context.Context, keep []string, at time.Time) (int, error) {
	q := s.db.WithContext(ctx).
		Model(&promo.PromoCode{}).
		Where("created_by = ?", CreatedBy).
		Where("valid_until IS NULL OR valid_until > ?", at)
	if len(keep) > 0 {
		q = q.Where("code NOT IN ?", keep)
	}

	res := q.Updates(map[string]any{"valid_until": at, "updated_at": at})
	if res.Error != nil {
		return 0, fmt.Errorf("consolepromo: expire withdrawn promo codes: %w", res.Error)
	}
	return int(res.RowsAffected), nil
}
