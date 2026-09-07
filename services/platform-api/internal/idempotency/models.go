// Package idempotency holds the IdempotencyKey model and its store.
//
// Ported from marketplace-api's internal/idempotency (mark8ly#720 Task 5)
// against the identical idempotency_keys shape (migration 0017 here mirrors
// marketplace-api's migration 000001 table verbatim), so this is the same
// code against the same schema, not a reinterpretation. Unlike
// marketplace-api, nothing here is wired to a cron sweep yet — this
// service's first (and so far only) consumer is the email-template PUT's
// Idempotency-Key replay, which needs Reserve/Lookup/Complete/Release, not
// the sweep. SweepExpired is still included so a future cron wiring is a
// one-line addition, not a second copy of this file.
package idempotency

import (
	"encoding/json"
	"time"
)

// IdempotencyKey stores a previously-seen request key plus its response,
// keyed by an Idempotency-Key header value.
//
// Response is json.RawMessage ([]byte underneath) rather than
// marketplace-api's gorm.io/datatypes.JSON, deliberately: pulling in that
// module for one column tagged jsonb would be a new dependency for a type
// GORM already round-trips correctly via the postgres driver's []byte
// handling plus the explicit column type tag below.
type IdempotencyKey struct {
	Key       string          `gorm:"primaryKey;column:key;type:varchar(255)" json:"key"`
	TenantID  string          `gorm:"column:tenant_id;type:uuid;not null"     json:"tenant_id"`
	Response  json.RawMessage `gorm:"column:response;type:jsonb"              json:"response,omitempty"`
	CreatedAt time.Time       `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	ExpiresAt time.Time       `gorm:"column:expires_at;not null"              json:"expires_at"`
}

func (IdempotencyKey) TableName() string { return "idempotency_keys" }
