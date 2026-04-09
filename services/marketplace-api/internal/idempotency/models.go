// Package idempotency holds the IdempotencyKey model. Cleanup of expired
// rows happens in the nightly sweep job (spec §14.10 — same CronJob as
// orphan GCS sweep). This package does not own the cleanup job itself,
// only the row shape.
package idempotency

import (
	"time"

	"gorm.io/datatypes"
)

// IdempotencyKey stores a previously-seen request key plus its response,
// keyed by an Idempotency-Key header value (see spec §13.2.7).
type IdempotencyKey struct {
	Key       string         `gorm:"primaryKey;column:key;type:varchar(255)" json:"key"`
	TenantID  string         `gorm:"column:tenant_id;type:uuid;not null"     json:"tenant_id"`
	Response  datatypes.JSON `gorm:"column:response;type:jsonb"              json:"response,omitempty"`
	CreatedAt time.Time      `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	ExpiresAt time.Time      `gorm:"column:expires_at;not null"              json:"expires_at"`
}

func (IdempotencyKey) TableName() string { return "idempotency_keys" }
