// Package apikeys implements §18.4 enterprise API keys: CSPRNG generation,
// bcrypt hashing, prefix-indexed lookup, scope enforcement, per-key rate
// limiting, and the bearer-auth middleware that gates the public R/W API.
package apikeys

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned by repo lookups when no row matches.
var ErrNotFound = errors.New("apikeys: not found")

// ScopeSet is a JSONB-backed slice of scope strings. Implements sql.Scanner +
// driver.Valuer so GORM round-trips it through PG jsonb columns.
type ScopeSet []string

// Scan implements sql.Scanner.
func (s *ScopeSet) Scan(v any) error {
	if v == nil {
		*s = nil
		return nil
	}
	switch t := v.(type) {
	case []byte:
		return json.Unmarshal(t, s)
	case string:
		return json.Unmarshal([]byte(t), s)
	default:
		return fmt.Errorf("apikeys: cannot scan %T into ScopeSet", v)
	}
}

// Value implements driver.Valuer.
func (s ScopeSet) Value() (driver.Value, error) { return json.Marshal(s) }

// Has reports whether the set contains the given scope.
func (s ScopeSet) Has(scope string) bool {
	return slices.Contains(s, scope)
}

// APIKey is the persisted form of an enterprise key (table: enterprise_api_keys).
type APIKey struct {
	ID               uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID         uuid.UUID  `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID          uuid.UUID  `gorm:"column:store_id;type:uuid;not null"`
	KeyPrefix        string     `gorm:"column:key_prefix;type:varchar(8);not null"`
	KeyHash          string     `gorm:"column:key_hash;type:varchar(60);not null"`
	Scopes           ScopeSet   `gorm:"column:scopes;type:jsonb;not null;default:'[]'"`
	RateLimitPerMin  int        `gorm:"column:rate_limit_per_min;not null;default:100"`
	Label            string     `gorm:"column:label;type:varchar(100);not null"`
	CreatedBy        uuid.UUID  `gorm:"column:created_by;type:uuid;not null"`
	CreatedAt        time.Time  `gorm:"column:created_at;not null;default:now()"`
	RevokedAt        *time.Time `gorm:"column:revoked_at"`
	RevokedReason    *string    `gorm:"column:revoked_reason;type:varchar(50)"`
	LastUsedAt       *time.Time `gorm:"column:last_used_at"`
	LastUsedIPHash   *string    `gorm:"column:last_used_ip_hash;type:varchar(64)"`
	RotationReplaces *uuid.UUID `gorm:"column:rotation_replaces;type:uuid"`
}

// TableName binds the GORM model to the migration-defined table.
func (APIKey) TableName() string { return "enterprise_api_keys" }

// IsUsable reports whether the key is currently valid for authentication.
// A key is usable when revoked_at is NULL or strictly in the future (24h
// rotation-overlap window per §18.4).
func (k APIKey) IsUsable(now time.Time) bool {
	return k.RevokedAt == nil || k.RevokedAt.After(now)
}
