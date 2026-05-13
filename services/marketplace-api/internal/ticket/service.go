package ticket

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ServiceConfig groups dependencies for the ticket service.
type ServiceConfig struct {
	DB     *gorm.DB
	Repo   Repository
	Logger *slog.Logger
	// Notifier is optional; when set, Service fires
	// NotifyTicketResolved on every status -> resolved transition.
	// Creation emails are fired from InternalHandler (so they only
	// happen for AI-escalated tickets, not merchant-opened ones).
	Notifier interface {
		NotifyTicketResolved(ctx context.Context, t *Ticket)
	}
}

// Service implements ticket CRUD and reply logic.
type Service struct {
	db     *gorm.DB
	repo   Repository
	logger *slog.Logger
	notify interface {
		NotifyTicketResolved(ctx context.Context, t *Ticket)
	}
}

// NewService constructs a ticket Service.
func NewService(cfg ServiceConfig) *Service {
	return &Service{
		db:     cfg.DB,
		repo:   cfg.Repo,
		logger: cfg.Logger,
		notify: cfg.Notifier,
	}
}

// List returns a paginated list of tickets for a store.
func (s *Service) List(ctx context.Context, f ListFilter) (ListResult, error) {
	return s.repo.List(ctx, s.db, f)
}

// Get returns a single ticket by ID with replies preloaded.
func (s *Service) Get(ctx context.Context, storeID, tenantID, id uuid.UUID) (*Ticket, error) {
	return s.repo.GetByID(ctx, s.db, storeID, tenantID, id)
}

// CreateInput holds the fields for creating a ticket.
type CreateInput struct {
	TenantID         uuid.UUID
	StoreID          uuid.UUID
	Subject          string
	Description      string
	Priority         string
	SubmittedByName  string
	SubmittedByEmail string
	// ConversationID is set when the ticket is auto-created from an
	// Otto chat that the customer escalated to a human. Empty when
	// the merchant opens the ticket manually from the admin UI.
	ConversationID string
}

// Create validates and persists a new ticket. The ticket number is
// generated atomically inside a transaction.
func (s *Service) Create(ctx context.Context, in CreateInput) (*Ticket, error) {
	if in.Subject == "" {
		return nil, apperrors.ValidationFailed("subject", "subject is required")
	}
	if len(in.Subject) > 300 {
		return nil, apperrors.ValidationFailed("subject", "subject must be 300 characters or fewer")
	}
	if in.Description == "" {
		return nil, apperrors.ValidationFailed("description", "description is required")
	}

	priority := TicketPriorityMedium
	if in.Priority != "" {
		if !ValidatePriority(in.Priority) {
			return nil, apperrors.ValidationFailed("priority", "priority must be low, medium, or high")
		}
		priority = TicketPriority(in.Priority)
	}

	now := time.Now()
	var created *Ticket

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		num, err := s.repo.NextTicketNumber(tx, in.StoreID)
		if err != nil {
			return err
		}

		t := &Ticket{
			TenantID:         in.TenantID,
			StoreID:          in.StoreID,
			TicketNumber:     num,
			Subject:          in.Subject,
			Description:      in.Description,
			Status:           TicketStatusOpen,
			Priority:         priority,
			SubmittedByName:  in.SubmittedByName,
			SubmittedByEmail: in.SubmittedByEmail,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if in.ConversationID != "" {
			cid := in.ConversationID
			t.ConversationID = &cid
		}
		if err := s.repo.Create(ctx, tx, t); err != nil {
			return err
		}
		created = t
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// GetByConversation returns the ticket created from a specific Otto
// conversation, or apperrors.NotFound if none exists. Used by the
// /internal/v1/tickets/from-conversation handler to short-circuit
// duplicate POSTs (slm-router retries on transient failure).
func (s *Service) GetByConversation(ctx context.Context, storeID uuid.UUID, conversationID string) (*Ticket, error) {
	return s.repo.GetByConversation(ctx, s.db, storeID, conversationID)
}

// UpdateStatus validates the status transition and persists it.
func (s *Service) UpdateStatus(ctx context.Context, storeID, tenantID, id uuid.UUID, newStatus string) (*Ticket, error) {
	if !ValidateStatus(newStatus) {
		return nil, apperrors.ValidationFailed("status", "status must be open, in_progress, resolved, or closed")
	}

	t, err := s.repo.GetByID(ctx, s.db, storeID, tenantID, id)
	if err != nil {
		return nil, err
	}

	target := TicketStatus(newStatus)
	if !t.CanTransitionTo(target) {
		return nil, apperrors.New(apperrors.CodeInvalidTransition,
			fmt.Sprintf("cannot transition from %s to %s", t.Status, target))
	}

	now := time.Now()
	t.Status = target
	t.UpdatedAt = now

	switch target {
	case TicketStatusResolved:
		t.ResolvedAt = &now
	case TicketStatusOpen, TicketStatusInProgress:
		// Any non-terminal reopen clears the resolved stamp so the UI
		// doesn't misleadingly show "resolved at ..." on an active
		// ticket.
		t.ResolvedAt = nil
	}

	if err := s.repo.UpdateStatus(ctx, s.db, t); err != nil {
		return nil, err
	}
	// Fire "ticket resolved" email out-of-band when applicable. SMTP
	// latency must not stretch the HTTP response.
	if s.notify != nil && target == TicketStatusResolved {
		go s.notify.NotifyTicketResolved(context.Background(), t)
	}
	return t, nil
}

// AddReplyInput holds the fields for adding a reply.
type AddReplyInput struct {
	StoreID     uuid.UUID
	TenantID    uuid.UUID
	TicketID    uuid.UUID
	AuthorType  string
	AuthorName  string
	AuthorEmail string
	Content     string
}

// AddReply validates and persists a new reply on a ticket.
func (s *Service) AddReply(ctx context.Context, in AddReplyInput) (*TicketReply, error) {
	if in.Content == "" {
		return nil, apperrors.ValidationFailed("content", "content is required")
	}
	if !ValidateAuthorType(in.AuthorType) {
		return nil, apperrors.ValidationFailed("author_type", "author_type must be merchant, platform, or customer")
	}

	// Verify ticket exists and belongs to the store/tenant, and is open
	// for new replies. Closed tickets are read-only; resolved tickets
	// can still receive replies (customers often have follow-ups).
	t, err := s.repo.GetByID(ctx, s.db, in.StoreID, in.TenantID, in.TicketID)
	if err != nil {
		return nil, err
	}
	if t.Status == TicketStatusClosed {
		return nil, apperrors.New(apperrors.CodeInvalidTransition,
			"cannot reply to a closed ticket")
	}

	var email *string
	if in.AuthorEmail != "" {
		email = &in.AuthorEmail
	}

	r := &TicketReply{
		TicketID:    in.TicketID,
		AuthorType:  AuthorType(in.AuthorType),
		AuthorName:  in.AuthorName,
		AuthorEmail: email,
		Content:     in.Content,
		CreatedAt:   time.Now(),
	}
	if err := s.repo.CreateReply(ctx, s.db, r); err != nil {
		return nil, err
	}

	// A reply on a resolved ticket flips it back to open so the
	// merchant's inbox shows the new activity. Best-effort — a
	// transition failure must not swallow the successful reply write.
	if t.Status == TicketStatusResolved {
		t.Status = TicketStatusOpen
		t.ResolvedAt = nil
		t.UpdatedAt = time.Now()
		_ = s.repo.UpdateStatus(ctx, s.db, t)
	}

	return r, nil
}

// ListForCustomer returns tickets submitted by a given customer email
// for a store, newest first. Scoped to store to prevent cross-store
// leakage even if the same email is used on another storefront.
func (s *Service) ListForCustomer(ctx context.Context, storeID uuid.UUID, email string) ([]Ticket, error) {
	return s.repo.ListByCustomerEmail(ctx, s.db, storeID, email)
}

// GetForCustomer returns a ticket owned by the given customer email
// (with replies preloaded), scoped to store. Returns NotFound if the
// ticket belongs to a different customer — enumeration guard.
func (s *Service) GetForCustomer(ctx context.Context, storeID, id uuid.UUID, email string) (*Ticket, error) {
	return s.repo.GetByIDForCustomerEmail(ctx, s.db, storeID, id, email)
}

// CountByStatus returns status counts for a store.
func (s *Service) CountByStatus(ctx context.Context, storeID, tenantID uuid.UUID) (map[string]int64, error) {
	return s.repo.CountByStatus(ctx, s.db, storeID, tenantID)
}
