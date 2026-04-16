// Package audit is marketplace-api's in-process audit log. It owns the
// audit_logs table, the emitter that writes to it, and the repository
// the admin handler reads from. The admin UI (Settings -> Audit Logs)
// reads the serialized shape produced by Entry.ToResponse().
package audit

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ActorType distinguishes user-initiated events from system/api events.
type ActorType string

const (
	ActorUser   ActorType = "user"
	ActorSystem ActorType = "system"
	ActorAPI    ActorType = "api"
)

// Status is the outcome of the audited action.
type Status string

const (
	StatusSuccess Status = "success"
	StatusFailure Status = "failure"
)

// Severity classifies the event for UI filtering and alerting.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Metadata is a free-form JSONB blob captured alongside each event.
// Stored as jsonb in Postgres; GORM marshals via the Scan/Value methods
// below. Keep entries small and flat for readable UI rendering.
type Metadata map[string]any

// Value implements driver.Valuer so GORM can persist Metadata as JSONB.
func (m Metadata) Value() (driver.Value, error) {
	if len(m) == 0 {
		return []byte(`{}`), nil
	}
	return json.Marshal(m)
}

// Scan implements sql.Scanner so GORM can load Metadata from JSONB.
func (m *Metadata) Scan(src any) error {
	if src == nil {
		*m = Metadata{}
		return nil
	}
	var raw []byte
	switch v := src.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return errors.New("audit.Metadata: unsupported scan source")
	}
	if len(raw) == 0 {
		*m = Metadata{}
		return nil
	}
	return json.Unmarshal(raw, m)
}

// Entry is the GORM model for an audit_logs row. Optional fields are
// pointers so we can write SQL NULL — important for ip_address which
// is an INET column and rejects empty strings.
type Entry struct {
	ID           uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID     uuid.UUID  `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID      uuid.UUID  `gorm:"column:store_id;type:uuid;not null"`
	ActorUserID  *uuid.UUID `gorm:"column:actor_user_id;type:uuid"`
	ActorEmail   *string    `gorm:"column:actor_email;type:text"`
	ActorType    ActorType  `gorm:"column:actor_type;type:varchar(16);not null"`
	Action       string     `gorm:"column:action;type:varchar(64);not null"`
	ResourceType string     `gorm:"column:resource_type;type:varchar(32);not null"`
	ResourceID   *string    `gorm:"column:resource_id;type:text"`
	Status       Status     `gorm:"column:status;type:varchar(16);not null"`
	Severity     Severity   `gorm:"column:severity;type:varchar(16);not null"`
	IPAddress    *string    `gorm:"column:ip_address;type:inet"`
	UserAgent    *string    `gorm:"column:user_agent;type:text"`
	Metadata     Metadata   `gorm:"column:metadata;type:jsonb;not null;default:'{}'::jsonb"`
	CreatedAt    time.Time  `gorm:"column:created_at;not null;default:now()"`
}

// TableName pins the Postgres table name so GORM's pluralizer can't drift.
func (Entry) TableName() string { return "audit_logs" }

// Response is the serialized shape consumed by apps/admin/lib/api/settings-tier2-api.ts
// (interface AuditLogEntry). Optional DB fields are flattened to "" so the
// frontend never has to handle nulls.
type Response struct {
	ID           string         `json:"id"`
	Timestamp    string         `json:"timestamp"`
	UserEmail    string         `json:"user_email"`
	ActorType    string         `json:"actor_type"` // "user" | "system" | "api" — UI labels system events
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	Status       string         `json:"status"`
	Severity     string         `json:"severity"`
	IPAddress    string         `json:"ip_address"`
	UserAgent    string         `json:"user_agent"`
	Metadata     map[string]any `json:"metadata"`
}

// ToResponse maps a DB row to the JSON shape the admin UI expects.
func (e Entry) ToResponse() Response {
	r := Response{
		ID:           e.ID.String(),
		Timestamp:    e.CreatedAt.UTC().Format(time.RFC3339),
		ActorType:    string(e.ActorType),
		Action:       e.Action,
		ResourceType: e.ResourceType,
		Status:       string(e.Status),
		Severity:     string(e.Severity),
		Metadata:     e.Metadata,
	}
	if e.ActorEmail != nil {
		r.UserEmail = *e.ActorEmail
	}
	if e.ResourceID != nil {
		r.ResourceID = *e.ResourceID
	}
	if e.IPAddress != nil {
		r.IPAddress = *e.IPAddress
	}
	if e.UserAgent != nil {
		r.UserAgent = *e.UserAgent
	}
	if r.Metadata == nil {
		r.Metadata = map[string]any{}
	}
	return r
}
