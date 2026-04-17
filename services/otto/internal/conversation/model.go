package conversation

import "time"

// Status is the lifecycle state of a conversation. The subset is
// deliberately small — "Otto v1" is customer ↔ staff only.
type Status string

const (
	// StatusPending — customer opened the thread, no staff has accepted yet.
	StatusPending Status = "pending"
	// StatusActive — a staff member has accepted and the thread is live.
	StatusActive Status = "active"
	// StatusClosed — staff resolved the thread. Read-only for customer.
	StatusClosed Status = "closed"
)

// Customer captures whoever opened the thread. For anonymous visitors
// session_token is the only durable identifier we have.
type Customer struct {
	SessionToken string `bson:"session_token" json:"session_token"`
	UserID       string `bson:"user_id,omitempty" json:"user_id,omitempty"`
	Name         string `bson:"name,omitempty" json:"name,omitempty"`
	Email        string `bson:"email,omitempty" json:"email,omitempty"`
}

// Assignee is the staff member currently handling the thread. Set on accept.
type Assignee struct {
	UserID    string    `bson:"user_id" json:"user_id"`
	Name      string    `bson:"name,omitempty" json:"name,omitempty"`
	Email     string    `bson:"email,omitempty" json:"email,omitempty"`
	AssignedAt time.Time `bson:"assigned_at" json:"assigned_at"`
}

// Conversation is a single support thread, fully scoped by tenant + store.
type Conversation struct {
	ID            string     `bson:"_id" json:"id"`
	TenantID      string     `bson:"tenant_id" json:"tenant_id"`
	StoreID       string     `bson:"store_id" json:"store_id"`
	Status        Status     `bson:"status" json:"status"`
	Subject       string     `bson:"subject,omitempty" json:"subject,omitempty"`
	Customer      Customer   `bson:"customer" json:"customer"`
	Assignee      *Assignee  `bson:"assignee,omitempty" json:"assignee,omitempty"`
	CreatedAt     time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt     time.Time  `bson:"updated_at" json:"updated_at"`
	LastMessageAt time.Time  `bson:"last_message_at" json:"last_message_at"`
	ClosedAt      *time.Time `bson:"closed_at,omitempty" json:"closed_at,omitempty"`

	// Denormalised counters for inbox UI.
	MessageCount        int `bson:"message_count" json:"message_count"`
	UnreadCountCustomer int `bson:"unread_count_customer" json:"unread_count_customer"`
	UnreadCountStaff    int `bson:"unread_count_staff" json:"unread_count_staff"`
}
