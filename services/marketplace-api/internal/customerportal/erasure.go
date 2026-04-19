package customerportal

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErasureRequest is the GORM model for customer_erasure_requests (migration 059).
// Append-only — no UPDATE path, no merchant-facing delete endpoint.
type ErasureRequest struct {
	ID            uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID      uuid.UUID  `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID       uuid.UUID  `gorm:"column:store_id;type:uuid;not null"`
	CustomerEmail string     `gorm:"column:customer_email;type:text;not null"`
	RequestedAt   time.Time  `gorm:"column:requested_at;not null;default:now()"`
	Status        string     `gorm:"column:status;type:text;not null;default:pending"`
	ProcessedAt   *time.Time `gorm:"column:processed_at"`
	Notes         *string    `gorm:"column:notes;type:text"`
}

// TableName returns the database table name for GORM.
func (ErasureRequest) TableName() string { return "customer_erasure_requests" }

// AppendErasureRequest inserts a new erasure request row. The unique constraint
// on (store_id, customer_email) means a duplicate returns a PG unique violation;
// callers should treat this as "already queued" and return 200/202.
func AppendErasureRequest(ctx context.Context, db *gorm.DB, req ErasureRequest) error {
	// ON CONFLICT DO NOTHING — duplicate is idempotent (§15.4 append-only spec).
	res := db.WithContext(ctx).Exec(`
		INSERT INTO customer_erasure_requests
		  (tenant_id, store_id, customer_email, status)
		VALUES (?, ?, ?, 'pending')
		ON CONFLICT (store_id, customer_email) DO NOTHING`,
		req.TenantID, req.StoreID, req.CustomerEmail,
	)
	if res.Error != nil {
		return fmt.Errorf("customerportal: append erasure request: %w", res.Error)
	}
	return nil
}
