package push

import (
	"time"

	"github.com/google/uuid"
)

type Token struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TenantID  uuid.UUID `gorm:"type:uuid;not null" json:"tenant_id"`
	StoreID   uuid.UUID `gorm:"type:uuid;not null" json:"store_id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null" json:"user_id"`
	DeviceID  string    `gorm:"type:varchar(100);not null" json:"device_id"`
	TokenStr  string    `gorm:"column:token;type:text;not null" json:"token"`
	Platform  string    `gorm:"type:varchar(10);not null" json:"platform"`
	CreatedAt time.Time `gorm:"not null;default:now()" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null;default:now()" json:"updated_at"`
}

func (Token) TableName() string { return "admin_push_tokens" }
