package personalisationupload

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Repository reads and writes personalisation_uploads.
//
// EVERY buyer-facing method takes cartToken and scopes on it. That is not
// defence in depth, it is the only thing standing between one shopper and
// another's photograph: an upload id is a bare uuid in a URL, and the
// storefront has no authenticated customer to check it against.
type Repository interface {
	Insert(ctx context.Context, u *Upload) error
	GetForCart(ctx context.Context, id, cartToken string) (*Upload, error)
	MarkVerified(ctx context.Context, id, cartToken string, size int64, contentType string, w, h *int) error
	SetCrop(ctx context.Context, id, cartToken string, previewKey string, crop []byte) error
	DeleteForCart(ctx context.Context, id, cartToken string) (*Upload, error)
	CountForCart(ctx context.Context, cartToken string) (int64, error)
	// ExtendLease pushes expires_at out to now()+TTL for an upload the
	// buyer is demonstrably still using (#966).
	ExtendLease(ctx context.Context, id, cartToken string, ttl time.Duration) error
	// ClaimExpired moves up to limit expired, unclaimed rows out of the
	// table and returns them so their objects can be destroyed.
	ClaimExpired(ctx context.Context, now time.Time, limit int) ([]Upload, error)
}

type gormRepository struct{ db *gorm.DB }

// NewRepository constructs a Repository bound to db.
func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) Insert(ctx context.Context, u *Upload) error {
	if err := r.db.WithContext(ctx).Create(u).Error; err != nil {
		return fmt.Errorf("personalisationupload: insert: %w", err)
	}
	return nil
}

// GetForCart loads one upload, scoped to the cart that owns it.
//
// Returns NotFound for an id belonging to another cart — never Forbidden.
// A distinguishable "exists but not yours" would turn the endpoint into an
// oracle for whether an upload id is real.
func (r *gormRepository) GetForCart(ctx context.Context, id, cartToken string) (*Upload, error) {
	var u Upload
	err := r.db.WithContext(ctx).
		Where("id = ? AND cart_token = ?", id, cartToken).
		First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("personalisation_upload")
	}
	if err != nil {
		return nil, fmt.Errorf("personalisationupload: get: %w", err)
	}
	return &u, nil
}

// MarkVerified records what the object actually turned out to be and
// flips pending -> verified. Scoped by cart, and by state so a claimed
// upload cannot be dragged back.
func (r *gormRepository) MarkVerified(
	ctx context.Context, id, cartToken string, size int64, contentType string, w, h *int,
) error {
	res := r.db.WithContext(ctx).Model(&Upload{}).
		Where("id = ? AND cart_token = ? AND state = ?", id, cartToken, StatePending).
		Updates(map[string]any{
			"state":        StateVerified,
			"size_bytes":   size,
			"content_type": contentType,
			"width_px":     w,
			"height_px":    h,
			"updated_at":   gorm.Expr("now()"),
		})
	if res.Error != nil {
		return fmt.Errorf("personalisationupload: mark verified: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("personalisation_upload")
	}
	return nil
}

// SetCrop records the rectangle and the preview key. The ORIGINAL is
// never touched: a re-crop re-derives from it, so the buyer can crop
// repeatedly without compounding loss.
func (r *gormRepository) SetCrop(ctx context.Context, id, cartToken, previewKey string, crop []byte) error {
	res := r.db.WithContext(ctx).Model(&Upload{}).
		Where("id = ? AND cart_token = ? AND state <> ?", id, cartToken, StateClaimed).
		Updates(map[string]any{
			"storage_key": previewKey,
			"crop":        crop,
			"updated_at":  gorm.Expr("now()"),
		})
	if res.Error != nil {
		return fmt.Errorf("personalisationupload: set crop: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("personalisation_upload")
	}
	return nil
}

// ExtendLease renews an upload's 72-hour lease.
//
// Called when the buyer looks at their own preview, which is the only
// evidence this service gets that a cart is still live. Carts sit in
// localStorage indefinitely while uploads are swept at 72 hours, so
// without this a shopper who takes four days to decide returns to a cart
// that renders fine and cannot be bought.
//
// Deliberately NOT applied to a claimed upload: an order owns it, the
// sweeper already ignores it, and moving its expiry would imply the
// lease still means something.
//
// A miss is not an error. The row may have been swept between the read
// and this write, and the caller's job — handing over a preview URL —
// does not depend on the renewal succeeding.
func (r *gormRepository) ExtendLease(ctx context.Context, id, cartToken string, ttl time.Duration) error {
	res := r.db.WithContext(ctx).Model(&Upload{}).
		Where("id = ? AND cart_token = ? AND state <> ?", id, cartToken, StateClaimed).
		Updates(map[string]any{
			"expires_at": gorm.Expr("now() + (? * interval '1 second')", int64(ttl.Seconds())),
			"updated_at": gorm.Expr("now()"),
		})
	if res.Error != nil {
		return fmt.Errorf("personalisationupload: extend lease: %w", res.Error)
	}
	return nil
}

// DeleteForCart removes an upload the buyer changed their mind about and
// returns it, so the caller can destroy its objects.
//
// A claimed upload is NOT deletable: an order depends on it.
func (r *gormRepository) DeleteForCart(ctx context.Context, id, cartToken string) (*Upload, error) {
	var u Upload
	err := r.db.WithContext(ctx).
		Where("id = ? AND cart_token = ? AND state <> ?", id, cartToken, StateClaimed).
		First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("personalisation_upload")
	}
	if err != nil {
		return nil, fmt.Errorf("personalisationupload: load for delete: %w", err)
	}
	if err := r.db.WithContext(ctx).Delete(&Upload{}, "id = ?", u.ID).Error; err != nil {
		return nil, fmt.Errorf("personalisationupload: delete: %w", err)
	}
	return &u, nil
}

// CountForCart backs the per-cart rate limit.
func (r *gormRepository) CountForCart(ctx context.Context, cartToken string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&Upload{}).
		Where("cart_token = ?", cartToken).Count(&n).Error
	if err != nil {
		return 0, fmt.Errorf("personalisationupload: count for cart: %w", err)
	}
	return n, nil
}

// ClaimExpired deletes up to limit expired, unclaimed rows and returns
// what it deleted.
//
// DELETE ... RETURNING, in one statement, so two sweeper replicas cannot
// both act on the same row: whichever deletes it gets the rows back, the
// other gets none. A select-then-delete would hand the same objects to
// both and produce duplicate delete calls against GCS.
func (r *gormRepository) ClaimExpired(ctx context.Context, now time.Time, limit int) ([]Upload, error) {
	var out []Upload
	err := r.db.WithContext(ctx).Raw(`
		DELETE FROM personalisation_uploads
		 WHERE id IN (
		   SELECT id FROM personalisation_uploads
		    WHERE state <> ? AND expires_at < ?
		    ORDER BY expires_at
		    LIMIT ?
		    FOR UPDATE SKIP LOCKED)
		RETURNING *`, StateClaimed, now, limit).Scan(&out).Error
	if err != nil {
		return nil, fmt.Errorf("personalisationupload: claim expired: %w", err)
	}
	return out, nil
}
