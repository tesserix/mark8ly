package page

import (
	"context"
	"regexp"
	"strings"

	"github.com/google/uuid"

	apperrors "github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// slugPattern matches URL-safe slugs: lowercase letters, digits, and
// internal hyphens. 3 to 63 chars total. Must start and end with
// alphanumeric (no leading/trailing/double hyphens).
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)

const (
	maxTitleLen          = 200
	maxSEOTitleLen       = 200
	maxSEODescriptionLen = 300
	maxBodyLen           = 200_000
)

// Service is the business-logic layer for pages. It enforces slug +
// length rules, defaults Published to true, and provides separate
// read paths for admin (may include drafts) and storefront
// (published-only).
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// CreateInput is the normalized input shape for Create.
type CreateInput struct {
	TenantID       uuid.UUID
	StoreID        uuid.UUID
	Slug           string
	Title          string
	Body           string
	SEOTitle       *string
	SEODescription *string
	Published      *bool // nil defaults to true
	SortOrder      int
}

// UpdateInput is the patch-style input for Update. All fields are
// optional; nil means "leave unchanged".
type UpdateInput struct {
	Slug           *string
	Title          *string
	Body           *string
	SEOTitle       *string
	SEODescription *string
	Published      *bool
	SortOrder      *int
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*Page, error) {
	in.Slug = strings.TrimSpace(in.Slug)
	in.Title = strings.TrimSpace(in.Title)
	if err := validateSlug(in.Slug); err != nil {
		return nil, err
	}
	if err := validateTitle(in.Title); err != nil {
		return nil, err
	}
	if err := validateSEO(in.SEOTitle, in.SEODescription); err != nil {
		return nil, err
	}
	if len(in.Body) > maxBodyLen {
		return nil, apperrors.ValidationFailed("body", "body exceeds 200 KB")
	}
	if in.TenantID == uuid.Nil {
		return nil, apperrors.ValidationFailed("tenant_id", "tenant id is required")
	}
	if in.StoreID == uuid.Nil {
		return nil, apperrors.ValidationFailed("store_id", "store id is required")
	}
	published := true
	if in.Published != nil {
		published = *in.Published
	}
	p := &Page{
		TenantID:       in.TenantID,
		StoreID:        in.StoreID,
		Slug:           in.Slug,
		Title:          in.Title,
		Body:           in.Body,
		SEOTitle:       in.SEOTitle,
		SEODescription: in.SEODescription,
		Published:      published,
		SortOrder:      in.SortOrder,
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (*Page, error) {
	patch := map[string]any{}
	if in.Slug != nil {
		slug := strings.TrimSpace(*in.Slug)
		if err := validateSlug(slug); err != nil {
			return nil, err
		}
		patch["slug"] = slug
	}
	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if err := validateTitle(title); err != nil {
			return nil, err
		}
		patch["title"] = title
	}
	if in.Body != nil {
		if len(*in.Body) > maxBodyLen {
			return nil, apperrors.ValidationFailed("body", "body exceeds 200 KB")
		}
		patch["body"] = *in.Body
	}
	if in.SEOTitle != nil {
		if len(*in.SEOTitle) > maxSEOTitleLen {
			return nil, apperrors.ValidationFailed("seo_title", "seo_title exceeds 200 chars")
		}
		patch["seo_title"] = *in.SEOTitle
	}
	if in.SEODescription != nil {
		if len(*in.SEODescription) > maxSEODescriptionLen {
			return nil, apperrors.ValidationFailed("seo_description", "seo_description exceeds 300 chars")
		}
		patch["seo_description"] = *in.SEODescription
	}
	if in.Published != nil {
		patch["published"] = *in.Published
	}
	if in.SortOrder != nil {
		patch["sort_order"] = *in.SortOrder
	}
	if len(patch) == 0 {
		return s.repo.GetByID(ctx, id)
	}
	if err := s.repo.Update(ctx, id, patch); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

// ListByStore returns pages for the given store. publishedOnly should
// be true for storefront callers; admin passes false to see drafts.
func (s *Service) ListByStore(ctx context.Context, storeID uuid.UUID, publishedOnly bool) ([]Page, error) {
	return s.repo.ListByStore(ctx, storeID, publishedOnly)
}

// GetByID is the admin read path (includes drafts).
func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*Page, error) {
	return s.repo.GetByID(ctx, id)
}

// GetBySlugForStorefront is the public read path — unpublished pages
// return nil, matching the 404 semantics the storefront needs.
func (s *Service) GetBySlugForStorefront(ctx context.Context, storeID uuid.UUID, slug string) (*Page, error) {
	return s.repo.GetBySlug(ctx, storeID, slug, true)
}

func validateSlug(slug string) error {
	if slug == "" {
		return apperrors.ValidationFailed("slug", "slug is required")
	}
	if !slugPattern.MatchString(slug) {
		return apperrors.ValidationFailed("slug",
			"slug must be 3-63 chars, lowercase alphanumeric + hyphens, starting and ending alphanumeric")
	}
	return nil
}

func validateTitle(title string) error {
	if title == "" {
		return apperrors.ValidationFailed("title", "title is required")
	}
	if len(title) > maxTitleLen {
		return apperrors.ValidationFailed("title", "title exceeds 200 chars")
	}
	return nil
}

func validateSEO(title, desc *string) error {
	if title != nil && len(*title) > maxSEOTitleLen {
		return apperrors.ValidationFailed("seo_title", "seo_title exceeds 200 chars")
	}
	if desc != nil && len(*desc) > maxSEODescriptionLen {
		return apperrors.ValidationFailed("seo_description", "seo_description exceeds 300 chars")
	}
	return nil
}
