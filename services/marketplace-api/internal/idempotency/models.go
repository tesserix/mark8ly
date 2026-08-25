// Package idempotency holds the IdempotencyKey model and its store.
//
// Expired rows are pruned by SweepExpired, wired onto the platform-admin
// daily cron (platformadmin.SweepSpec) since #286 — this table's first
// consumer. An earlier version of this comment, and the one in migration
// 000001, both claimed a pre-existing nightly sweep handled it. Neither was
// true: the only other references delete by tenant_id when a tenant is
// hard-deleted or purged, and nothing read expires_at at all.
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
