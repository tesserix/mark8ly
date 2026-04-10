package order

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ReturnStatus is the return lifecycle. requested → approved → received →
// refunded is the happy path; requested → rejected is the side path.
type ReturnStatus string

const (
	ReturnStatusRequested ReturnStatus = "requested"
	ReturnStatusApproved  ReturnStatus = "approved"
	ReturnStatusReceived  ReturnStatus = "received"
	ReturnStatusRefunded  ReturnStatus = "refunded"
	ReturnStatusRejected  ReturnStatus = "rejected"
)

var returnStatusTransitions = map[ReturnStatus][]ReturnStatus{
	ReturnStatusRequested: {ReturnStatusApproved, ReturnStatusRejected},
	ReturnStatusApproved:  {ReturnStatusReceived},
	ReturnStatusReceived:  {ReturnStatusRefunded},
	ReturnStatusRefunded:  nil, // terminal
	ReturnStatusRejected:  nil, // terminal
}

func (s ReturnStatus) CanTransitionTo(target ReturnStatus) bool {
	allowed, ok := returnStatusTransitions[s]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == target {
			return true
		}
	}
	return false
}

// -----------------------------------------------------------------------------
// Repository
// -----------------------------------------------------------------------------

// ReturnRepository is the data-access surface for the return aggregate.
type ReturnRepository interface {
	CreateInTx(tx *gorm.DB, r *Return, items []ReturnItem) error
	GetByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*Return, []ReturnItem, error)
	UpdateStatus(tx *gorm.DB, id uuid.UUID, target ReturnStatus) error
	StampReceived(tx *gorm.DB, id uuid.UUID) error
	StampRefunded(tx *gorm.DB, id uuid.UUID, amount decimal.Decimal) error
}

type gormReturnRepository struct{}

// NewReturnRepository constructs a stateless repository.
func NewReturnRepository() ReturnRepository { return &gormReturnRepository{} }

func (gormReturnRepository) CreateInTx(tx *gorm.DB, r *Return, items []ReturnItem) error {
	if err := tx.Create(r).Error; err != nil {
		return err
	}
	for i := range items {
		items[i].ReturnID = r.ID
	}
	if len(items) > 0 {
		if err := tx.Create(&items).Error; err != nil {
			return err
		}
	}
	return nil
}

func (gormReturnRepository) GetByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*Return, []ReturnItem, error) {
	var r Return
	if err := db.WithContext(ctx).Where("id = ?", id).First(&r).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, apperrors.NotFound("return")
		}
		return nil, nil, err
	}
	var items []ReturnItem
	if err := db.WithContext(ctx).Where("return_id = ?", id).Find(&items).Error; err != nil {
		return nil, nil, err
	}
	return &r, items, nil
}

func (gormReturnRepository) UpdateStatus(tx *gorm.DB, id uuid.UUID, target ReturnStatus) error {
	res := tx.Model(&Return{}).
		Where("id = ?", id).
		Updates(map[string]any{"status": string(target), "updated_at": gorm.Expr("now()")})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("return")
	}
	return nil
}

func (gormReturnRepository) StampReceived(tx *gorm.DB, id uuid.UUID) error {
	return tx.Model(&Return{}).Where("id = ?", id).
		Update("received_at", gorm.Expr("now()")).Error
}

func (gormReturnRepository) StampRefunded(tx *gorm.DB, id uuid.UUID, amount decimal.Decimal) error {
	return tx.Model(&Return{}).Where("id = ?", id).
		Updates(map[string]any{
			"refunded_at":   gorm.Expr("now()"),
			"refund_amount": amount,
		}).Error
}

// -----------------------------------------------------------------------------
// Service
// -----------------------------------------------------------------------------

// ReturnService orchestrates the return lifecycle. It depends on order.Service
// for cross-module event recording (RecordReturnEvent) — every state change
// here writes a corresponding row to order_events + outbox_events via the
// orders module, in the same tx.
type ReturnService struct {
	db        *gorm.DB
	repo      ReturnRepository
	orderRepo Repository
	orderSvc  *Service
	outbox    outbox.Repository
}

// NewReturnService constructs a ReturnService.
func NewReturnService(db *gorm.DB, repo ReturnRepository, orderRepo Repository, orderSvc *Service, outboxRepo outbox.Repository) *ReturnService {
	return &ReturnService{db: db, repo: repo, orderRepo: orderRepo, orderSvc: orderSvc, outbox: outboxRepo}
}

// RequestInput is the storefront-side payload that creates a return request.
type RequestInput struct {
	TenantID       uuid.UUID
	StoreID        uuid.UUID
	StorePrefix    string
	ReturnSeq      int64 // from order.NextDocumentNumber(..., "return")
	OrderID        uuid.UUID
	Reason         *string
	Notes          *string
	Items          []ReturnItem // OrderItemID + Quantity (+ optional Reason) per line
	CurrencyCode   string
	RequestedAt    time.Time
}

// Request creates a new return in 'requested' state. Validates that each
// requested item exists on the order and that quantity does not exceed the
// ordered quantity.
func (s *ReturnService) Request(ctx context.Context, in RequestInput) (*Return, []ReturnItem, error) {
	if in.RequestedAt.IsZero() {
		in.RequestedAt = time.Now().UTC()
	}

	// Pull the order's items so we can validate quantities.
	_, orderItems, _, err := s.orderRepo.GetByID(ctx, s.db, in.OrderID)
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[uuid.UUID]OrderItem, len(orderItems))
	for _, oi := range orderItems {
		byID[oi.ID] = oi
	}
	for _, ri := range in.Items {
		oi, ok := byID[ri.OrderItemID]
		if !ok {
			return nil, nil, apperrors.NotFound("order_item")
		}
		if ri.Quantity > oi.Quantity {
			return nil, nil, apperrors.ReturnItemsExceedOrdered(oi.SKUSnapshot, ri.Quantity, oi.Quantity)
		}
	}

	number := FormatReturnNumber(in.StorePrefix, in.RequestedAt, in.ReturnSeq)
	r := &Return{
		TenantID:     in.TenantID,
		StoreID:      in.StoreID,
		OrderID:      in.OrderID,
		ReturnNumber: number,
		Status:       string(ReturnStatusRequested),
		Reason:       in.Reason,
		Notes:        in.Notes,
		CurrencyCode: in.CurrencyCode,
		RequestedAt:  in.RequestedAt,
	}

	var (
		created      *Return
		createdItems []ReturnItem
	)
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.repo.CreateInTx(tx, r, in.Items); err != nil {
			return err
		}
		// Cross-module: record on order_events + outbox_events.
		if err := s.orderSvc.RecordReturnEvent(ctx, tx, in.OrderID, EventKindReturnRequested, outbox.EventReturnRequested, ReturnEventPayload{
			ReturnID:     r.ID.String(),
			ReturnNumber: number,
			Reason:       derefString(in.Reason),
		}); err != nil {
			return err
		}
		created = r
		createdItems = in.Items
		return nil
	})
	return created, createdItems, err
}

// Approve transitions a return from requested → approved.
func (s *ReturnService) Approve(ctx context.Context, returnID uuid.UUID) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r, _, err := s.repo.GetByID(ctx, tx, returnID)
		if err != nil {
			return err
		}
		if !ReturnStatus(r.Status).CanTransitionTo(ReturnStatusApproved) {
			return apperrors.InvalidTransition("return", r.Status, string(ReturnStatusApproved))
		}
		if err := s.repo.UpdateStatus(tx, returnID, ReturnStatusApproved); err != nil {
			return err
		}
		return s.orderSvc.RecordReturnEvent(ctx, tx, r.OrderID, EventKindReturnApproved, outbox.EventReturnApproved, ReturnEventPayload{
			ReturnID:     r.ID.String(),
			ReturnNumber: r.ReturnNumber,
		})
	})
}

// MarkReceived transitions approved → received and stamps received_at.
func (s *ReturnService) MarkReceived(ctx context.Context, returnID uuid.UUID) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r, _, err := s.repo.GetByID(ctx, tx, returnID)
		if err != nil {
			return err
		}
		if !ReturnStatus(r.Status).CanTransitionTo(ReturnStatusReceived) {
			return apperrors.InvalidTransition("return", r.Status, string(ReturnStatusReceived))
		}
		if err := s.repo.UpdateStatus(tx, returnID, ReturnStatusReceived); err != nil {
			return err
		}
		if err := s.repo.StampReceived(tx, returnID); err != nil {
			return err
		}
		return s.orderSvc.RecordReturnEvent(ctx, tx, r.OrderID, EventKindReturnReceived, outbox.EventReturnReceived, ReturnEventPayload{
			ReturnID:     r.ID.String(),
			ReturnNumber: r.ReturnNumber,
		})
	})
}

// MarkRefunded transitions received → refunded, stamps refunded_at, AND
// calls order.Service.RecordRefund inside the same tx so the orders.refunded_amount
// is updated atomically alongside the return state change.
//
// paymentStatusTarget is whichever PaymentStatus the caller wants the
// underlying order to land on (PartiallyRefunded for partial, Refunded for full).
func (s *ReturnService) MarkRefunded(ctx context.Context, returnID uuid.UUID, amount decimal.Decimal, paymentStatusTarget PaymentStatus, reason string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r, _, err := s.repo.GetByID(ctx, tx, returnID)
		if err != nil {
			return err
		}
		if !ReturnStatus(r.Status).CanTransitionTo(ReturnStatusRefunded) {
			return apperrors.InvalidTransition("return", r.Status, string(ReturnStatusRefunded))
		}
		if err := s.repo.UpdateStatus(tx, returnID, ReturnStatusRefunded); err != nil {
			return err
		}
		if err := s.repo.StampRefunded(tx, returnID, amount); err != nil {
			return err
		}
		// Cross-module atomic refund.
		if err := s.orderSvc.RecordRefund(ctx, tx, r.OrderID, amount, paymentStatusTarget, reason); err != nil {
			return err
		}
		return s.orderSvc.RecordReturnEvent(ctx, tx, r.OrderID, EventKindReturnRefunded, outbox.EventReturnRefunded, ReturnEventPayload{
			ReturnID:     r.ID.String(),
			ReturnNumber: r.ReturnNumber,
			Amount:       amount.String(),
			Reason:       reason,
		})
	})
}

// Reject transitions requested → rejected.
func (s *ReturnService) Reject(ctx context.Context, returnID uuid.UUID, reason string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		r, _, err := s.repo.GetByID(ctx, tx, returnID)
		if err != nil {
			return err
		}
		if !ReturnStatus(r.Status).CanTransitionTo(ReturnStatusRejected) {
			return apperrors.InvalidTransition("return", r.Status, string(ReturnStatusRejected))
		}
		if err := s.repo.UpdateStatus(tx, returnID, ReturnStatusRejected); err != nil {
			return err
		}
		return s.orderSvc.RecordReturnEvent(ctx, tx, r.OrderID, EventKindReturnRejected, outbox.EventReturnRejected, ReturnEventPayload{
			ReturnID:     r.ID.String(),
			ReturnNumber: r.ReturnNumber,
			Reason:       reason,
		})
	})
}

// -----------------------------------------------------------------------------
// internal helpers
// -----------------------------------------------------------------------------

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

