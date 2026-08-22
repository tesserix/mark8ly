package platformadmin

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// SweepSpec is the cron expression for the expired nonce sweep — runs at
// 09:45 UTC daily, a free slot alongside the other daily crons (00:15
// expiry, 02:00 audit prune, 09:00 banner, 09:15 SCA, 09:30 trial).
const SweepSpec = "45 9 * * *"

// SweepExpiredNonces deletes platform_request_nonces rows past their
// expires_at. Safe by construction: expires_at is set to signedTS + window,
// exactly the instant a request stops being signature-valid (see
// RequirePlatformAuth), so a row can never be deleted while the request it
// guards is still replayable.
func SweepExpiredNonces(ctx context.Context, db *gorm.DB) (int64, error) {
	res := db.WithContext(ctx).
		Where("expires_at < now()").
		Delete(&Nonce{})
	if res.Error != nil {
		return 0, fmt.Errorf("platformadmin: sweep expired nonces: %w", res.Error)
	}
	return res.RowsAffected, nil
}
