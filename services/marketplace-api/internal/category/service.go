// Package category — service layer for the per-store category tree.
//
// Create/Update/Delete run inside a DB transaction and enqueue an
// outbox_events row in the same tx so the publisher (M3 Task 5) sees
// domain events atomically with the mutation. Every payload contains
// the top-level "store_id" key required by the publisher's watermark
// upsert.
package category

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Service orchestrates category mutations and their outbox events.
type Service struct {
	db         *gorm.DB
	repo       Repository
	outboxRepo outbox.Repository
	logger     *slog.Logger
}

// Config bundles Service dependencies.
type Config struct {
	DB         *gorm.DB
	Repo       Repository
	OutboxRepo outbox.Repository
	Logger     *slog.Logger
}

// NewService builds a Service from cfg.
func NewService(cfg Config) *Service {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		db:         cfg.DB,
		repo:       cfg.Repo,
		outboxRepo: cfg.OutboxRepo,
		logger:     logger,
	}
}

// CreateRequest is the service-level DTO for category creation.
type CreateRequest struct {
	StoreID     string
	TenantID    string
	ParentID    *string
	Name        string
	Slug        string // optional; generated from Name if empty
	Description *string
	ImageURL    *string
	Position    int
	IsActive    bool
	Featured    bool
}

// UpdateRequest is the service-level DTO for category update; only
// non-nil fields are applied.
type UpdateRequest struct {
	ID          string
	StoreID     string
	TenantID    string
	ParentID    *string
	Name        *string
	Slug        *string
	Description *string
	ImageURL    *string
	Position    *int
	IsActive    *bool
	Featured    *bool
}

// Create inserts a new category and enqueues a category.created outbox
// event in the same transaction.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Category, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, apperrors.ValidationFailed("name", "name must not be empty")
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = generateSlug(name)
	}
	if slug == "" {
		return nil, apperrors.ValidationFailed("slug", "slug could not be generated from name")
	}

	cat := &Category{
		TenantID:    req.TenantID,
		StoreID:     req.StoreID,
		ParentID:    req.ParentID,
		Name:        name,
		Slug:        slug,
		Description: req.Description,
		ImageURL:    req.ImageURL,
		Position:    req.Position,
		IsActive:    req.IsActive,
		Featured:    req.Featured,
	}

	// NOTE: Repository.Create is kept for future non-tx callers, but for
	// atomicity with the outbox enqueue we insert directly on the tx and
	// translate the unique-slug violation inline.
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Create(cat).Error; err != nil {
			return translateCreateErr(err, slug)
		}
		payload, err := json.Marshal(map[string]any{
			"store_id":    req.StoreID,
			"category_id": cat.ID,
		})
		if err != nil {
			return fmt.Errorf("category: marshal created payload: %w", err)
		}
		evt := &outbox.OutboxEvent{
			TenantID:    req.TenantID,
			Aggregate:   outbox.AggregateCategory,
			AggregateID: cat.ID,
			EventType:   outbox.EventCategoryCreated,
			Payload:     datatypes.JSON(payload),
		}
		return s.outboxRepo.EnqueueInTx(ctx, tx, evt)
	})
	if err != nil {
		return nil, err
	}
	return cat, nil
}

// Update applies a partial mutation and enqueues a category.updated
// outbox event.
func (s *Service) Update(ctx context.Context, req UpdateRequest) (*Category, error) {
	// Pre-check existence so callers get a clean NotFound rather than a
	// "0 rows affected" from an empty update.
	if _, err := s.repo.GetByIDForStore(ctx, req.ID, req.StoreID, req.TenantID); err != nil {
		return nil, err
	}

	fields := map[string]any{}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, apperrors.ValidationFailed("name", "name must not be empty")
		}
		fields["name"] = name
	}
	if req.Slug != nil {
		slug := strings.TrimSpace(*req.Slug)
		if slug == "" {
			return nil, apperrors.ValidationFailed("slug", "slug must not be empty")
		}
		fields["slug"] = slug
	}
	if req.Description != nil {
		fields["description"] = *req.Description
	}
	if req.ImageURL != nil {
		fields["image_url"] = *req.ImageURL
	}
	if req.Position != nil {
		fields["position"] = *req.Position
	}
	if req.IsActive != nil {
		fields["is_active"] = *req.IsActive
	}
	if req.Featured != nil {
		fields["featured"] = *req.Featured
	}
	if req.ParentID != nil {
		fields["parent_id"] = *req.ParentID
	}

	if len(fields) > 0 {
		fields["updated_at"] = gorm.Expr("now()")
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(fields) > 0 {
			if err := s.repo.UpdateInTx(ctx, tx, req.ID, req.StoreID, req.TenantID, fields); err != nil {
				return err
			}
		}
		payload, err := json.Marshal(map[string]any{
			"store_id":    req.StoreID,
			"category_id": req.ID,
		})
		if err != nil {
			return fmt.Errorf("category: marshal updated payload: %w", err)
		}
		evt := &outbox.OutboxEvent{
			TenantID:    req.TenantID,
			Aggregate:   outbox.AggregateCategory,
			AggregateID: req.ID,
			EventType:   outbox.EventCategoryUpdated,
			Payload:     datatypes.JSON(payload),
		}
		return s.outboxRepo.EnqueueInTx(ctx, tx, evt)
	})
	if err != nil {
		return nil, err
	}

	return s.repo.GetByIDForStore(ctx, req.ID, req.StoreID, req.TenantID)
}

// Delete soft-deletes a category after refusing when it still has
// children or products. Enqueues a category.deleted outbox event on
// success.
func (s *Service) Delete(ctx context.Context, id, storeID, tenantID string) error {
	// Pre-checks outside the tx: these are read-only and let us fail
	// fast with a precise typed error before opening a write tx.
	if _, err := s.repo.GetByIDForStore(ctx, id, storeID, tenantID); err != nil {
		return err
	}
	if n, err := s.repo.HasChildren(ctx, id); err != nil {
		return err
	} else if n > 0 {
		return apperrors.CategoryHasChildren(n)
	}
	if n, err := s.repo.HasProducts(ctx, id); err != nil {
		return err
	} else if n > 0 {
		return apperrors.CategoryNotEmpty(n)
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.repo.SoftDeleteInTx(ctx, tx, id, storeID, tenantID); err != nil {
			return err
		}
		payload, err := json.Marshal(map[string]any{
			"store_id":    storeID,
			"category_id": id,
		})
		if err != nil {
			return fmt.Errorf("category: marshal deleted payload: %w", err)
		}
		evt := &outbox.OutboxEvent{
			TenantID:    tenantID,
			Aggregate:   outbox.AggregateCategory,
			AggregateID: id,
			EventType:   outbox.EventCategoryDeleted,
			Payload:     datatypes.JSON(payload),
		}
		return s.outboxRepo.EnqueueInTx(ctx, tx, evt)
	})
}

// translateCreateErr mirrors repository.isUniqueSlug so the service's
// direct-on-tx insert path returns the same typed error as the repo.
func translateCreateErr(err error, slug string) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" &&
		pgErr.ConstraintName == "categories_slug_per_store_live_unique" {
		return apperrors.SlugTaken(slug, slug+"-2")
	}
	return fmt.Errorf("category: create: %w", err)
}

// generateSlug produces a naïve, pure slug from a display name:
// lowercase, ascii alphanumerics, spaces/dashes/underscores collapse
// to single dashes, trimmed and capped at 200 chars.
func generateSlug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	var lastDash bool
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '-' || r == '_':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if len(out) > 200 {
		out = out[:200]
	}
	return out
}
