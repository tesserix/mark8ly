package admin

import (
	"github.com/mark8ly/marketplace-api/internal/ticket"
)

// CreateTicketRequest is the wire request for POST .../tickets.
type CreateTicketRequest struct {
	Subject     string `json:"subject" binding:"required"`
	Description string `json:"description" binding:"required"`
	Priority    string `json:"priority"`
}

// TicketReplyRequest is the wire request for POST .../tickets/:id/reply.
type TicketReplyRequest struct {
	Content string `json:"content" binding:"required"`
}

// UpdateTicketStatusRequest is the wire request for PATCH .../tickets/:id.
type UpdateTicketStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

// TicketResponse is the wire DTO for a ticket.
type TicketResponse struct {
	ID               string                `json:"id"`
	TicketNumber     string                `json:"ticket_number"`
	Subject          string                `json:"subject"`
	Description      string                `json:"description"`
	Status           string                `json:"status"`
	Priority         string                `json:"priority"`
	SubmittedByName  string                `json:"submitted_by_name"`
	SubmittedByEmail string                `json:"submitted_by_email"`
	ResolvedAt       *string               `json:"resolved_at,omitempty"`
	CreatedAt        string                `json:"created_at"`
	UpdatedAt        string                `json:"updated_at"`
	Replies          []TicketReplyResponse `json:"replies"`
}

// TicketReplyResponse is the wire DTO for a ticket reply.
type TicketReplyResponse struct {
	ID          string  `json:"id"`
	AuthorType  string  `json:"author_type"`
	AuthorName  string  `json:"author_name"`
	AuthorEmail *string `json:"author_email,omitempty"`
	Content     string  `json:"content"`
	CreatedAt   string  `json:"created_at"`
}

func toTicketResponse(t ticket.Ticket) TicketResponse {
	resp := TicketResponse{
		ID:               t.ID.String(),
		TicketNumber:     t.TicketNumber,
		Subject:          t.Subject,
		Description:      t.Description,
		Status:           string(t.Status),
		Priority:         string(t.Priority),
		SubmittedByName:  t.SubmittedByName,
		SubmittedByEmail: t.SubmittedByEmail,
		CreatedAt:        t.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:        t.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if t.ResolvedAt != nil {
		s := t.ResolvedAt.Format("2006-01-02T15:04:05Z")
		resp.ResolvedAt = &s
	}
	// Always initialize to a non-nil slice so the JSON response emits
	// an empty `[]` instead of omitting the key — otherwise the admin
	// UI crashes on `ticket.replies.length` when a ticket has no
	// replies yet.
	resp.Replies = make([]TicketReplyResponse, 0, len(t.Replies))
	for _, r := range t.Replies {
		resp.Replies = append(resp.Replies, toTicketReplyResponse(r))
	}
	return resp
}

func toTicketReplyResponse(r ticket.TicketReply) TicketReplyResponse {
	resp := TicketReplyResponse{
		ID:         r.ID.String(),
		AuthorType: string(r.AuthorType),
		AuthorName: r.AuthorName,
		Content:    r.Content,
		CreatedAt:  r.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if r.AuthorEmail != nil {
		resp.AuthorEmail = r.AuthorEmail
	}
	return resp
}
