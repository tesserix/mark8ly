// Package ticket implements support ticket CRUD and reply management
// for Dashboard D2.
package ticket

import (
	"time"

	"github.com/google/uuid"
)

// TicketStatus enumerates ticket lifecycle states.
type TicketStatus string

const (
	// TicketStatusOpen is the initial state — the merchant hasn't
	// started working on the ticket yet.
	TicketStatusOpen TicketStatus = "open"
	// TicketStatusInProgress signals the merchant has picked up the
	// ticket and is actively working on it. Visible to the customer
	// so they know they're not being ignored.
	TicketStatusInProgress TicketStatus = "in_progress"
	// TicketStatusResolved means the merchant believes they've
	// answered the question. A new customer reply auto-reopens the
	// ticket to `open` so follow-ups aren't lost.
	TicketStatusResolved TicketStatus = "resolved"
	// TicketStatusClosed is terminal — no more replies accepted.
	// Use for archived conversations that don't need any follow-up.
	TicketStatusClosed TicketStatus = "closed"
)

// ValidateStatus returns true if s is a valid ticket status.
func ValidateStatus(s string) bool {
	switch TicketStatus(s) {
	case TicketStatusOpen, TicketStatusInProgress,
		TicketStatusResolved, TicketStatusClosed:
		return true
	}
	return false
}

// TicketPriority enumerates ticket priority levels.
type TicketPriority string

const (
	TicketPriorityLow    TicketPriority = "low"
	TicketPriorityMedium TicketPriority = "medium"
	TicketPriorityHigh   TicketPriority = "high"
)

// ValidatePriority returns true if p is a valid ticket priority.
func ValidatePriority(p string) bool {
	switch TicketPriority(p) {
	case TicketPriorityLow, TicketPriorityMedium, TicketPriorityHigh:
		return true
	}
	return false
}

// AuthorType enumerates who authored a ticket reply.
type AuthorType string

const (
	AuthorTypeMerchant AuthorType = "merchant"
	AuthorTypePlatform AuthorType = "platform"
	AuthorTypeCustomer AuthorType = "customer"
)

// ValidateAuthorType returns true if a is a valid author type.
func ValidateAuthorType(a string) bool {
	switch AuthorType(a) {
	case AuthorTypeMerchant, AuthorTypePlatform, AuthorTypeCustomer:
		return true
	}
	return false
}

// Ticket is the GORM model for the tickets table.
type Ticket struct {
	ID               uuid.UUID    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID         uuid.UUID    `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID          uuid.UUID    `gorm:"column:store_id;type:uuid;not null"`
	TicketNumber     string       `gorm:"column:ticket_number;type:varchar(20);not null"`
	Subject          string       `gorm:"column:subject;type:varchar(300);not null"`
	Description      string       `gorm:"column:description;type:text;not null"`
	Status           TicketStatus `gorm:"column:status;type:varchar(20);not null;default:open"`
	Priority         TicketPriority `gorm:"column:priority;type:varchar(10);not null;default:medium"`
	SubmittedByName  string       `gorm:"column:submitted_by_name;type:varchar(200);not null"`
	SubmittedByEmail string       `gorm:"column:submitted_by_email;type:varchar(300);not null"`
	ResolvedAt       *time.Time   `gorm:"column:resolved_at"`
	CreatedAt        time.Time    `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt        time.Time    `gorm:"column:updated_at;not null;default:now()"`

	// Preloaded association.
	Replies []TicketReply `gorm:"foreignKey:TicketID"`
}

func (Ticket) TableName() string { return "tickets" }

// CanTransitionTo returns true if moving from the ticket's current status
// to the target status is a valid transition. Same target (noop) is
// always rejected so the UI doesn't accidentally churn updated_at.
func (t *Ticket) CanTransitionTo(target TicketStatus) bool {
	if target == t.Status {
		return false
	}
	switch t.Status {
	case TicketStatusOpen:
		return target == TicketStatusInProgress ||
			target == TicketStatusResolved ||
			target == TicketStatusClosed
	case TicketStatusInProgress:
		return target == TicketStatusOpen ||
			target == TicketStatusResolved ||
			target == TicketStatusClosed
	case TicketStatusResolved:
		return target == TicketStatusOpen ||
			target == TicketStatusInProgress ||
			target == TicketStatusClosed
	case TicketStatusClosed:
		return false // terminal
	}
	return false
}

// TicketReply is the GORM model for the ticket_replies table.
type TicketReply struct {
	ID          uuid.UUID  `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TicketID    uuid.UUID  `gorm:"column:ticket_id;type:uuid;not null"`
	AuthorType  AuthorType `gorm:"column:author_type;type:varchar(20);not null"`
	AuthorName  string     `gorm:"column:author_name;type:varchar(200);not null"`
	AuthorEmail *string    `gorm:"column:author_email;type:varchar(300)"`
	Content     string     `gorm:"column:content;type:text;not null"`
	CreatedAt   time.Time  `gorm:"column:created_at;not null;default:now()"`
}

func (TicketReply) TableName() string { return "ticket_replies" }
