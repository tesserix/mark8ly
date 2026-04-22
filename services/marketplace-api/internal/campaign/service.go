package campaign

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ServiceConfig bundles service dependencies.
type ServiceConfig struct {
	DB            *gorm.DB
	Repo          Repository
	SegmentEngine *SegmentEngine
	Logger        *slog.Logger
}

// Service is the campaign business logic layer.
type Service struct {
	db            *gorm.DB
	repo          Repository
	segmentEngine *SegmentEngine
	logger        *slog.Logger
}

// NewService constructs a campaign Service.
func NewService(cfg ServiceConfig) *Service {
	return &Service{
		db:            cfg.DB,
		repo:          cfg.Repo,
		segmentEngine: cfg.SegmentEngine,
		logger:        cfg.Logger,
	}
}

// DB returns the underlying *gorm.DB for read-only handler access.
func (s *Service) DB() *gorm.DB { return s.db }

// CreateCampaign validates, sanitizes content, and persists a new campaign.
func (s *Service) CreateCampaign(ctx context.Context, c *Campaign) error {
	if c.Name == "" {
		return apperrors.ValidationFailed("name", "name is required")
	}
	if c.TenantID == uuid.Nil {
		return apperrors.ValidationFailed("tenant_id", "tenant_id is required")
	}
	if c.StoreID == uuid.Nil {
		return apperrors.ValidationFailed("store_id", "store_id is required")
	}

	// Sanitize content before storage.
	if c.Content != nil && *c.Content != "" {
		sanitized := product.Sanitize(*c.Content)
		c.Content = &sanitized
	}

	// Default type and status.
	if c.Type == "" {
		c.Type = TypeEmail
	}
	c.Status = StatusDraft

	return s.repo.CreateCampaign(s.db, c)
}

// UpdateCampaign validates the campaign is in draft status and applies changes.
func (s *Service) UpdateCampaign(ctx context.Context, c *Campaign) error {
	existing, err := s.repo.GetCampaignByID(ctx, s.db, c.ID)
	if err != nil {
		return err
	}
	if existing.Status != StatusDraft {
		return apperrors.New(apperrors.CodeCampaignNotDraft, "campaign must be in draft status to update")
	}

	// Sanitize content before storage.
	if c.Content != nil && *c.Content != "" {
		sanitized := product.Sanitize(*c.Content)
		c.Content = &sanitized
	}

	return s.repo.UpdateCampaign(s.db, c)
}

// ScheduleCampaign resolves the segment, creates recipients, and sets the
// campaign to "scheduled" with a future send time.
func (s *Service) ScheduleCampaign(ctx context.Context, campaignID uuid.UUID, scheduledAt time.Time) (*Campaign, error) {
	if scheduledAt.Before(time.Now()) {
		return nil, apperrors.New(apperrors.CodeCampaignSchedulePast, "scheduled_at must be in the future")
	}

	c, err := s.repo.GetCampaignByID(ctx, s.db, campaignID)
	if err != nil {
		return nil, err
	}
	if c.Status != StatusDraft {
		return nil, apperrors.New(apperrors.CodeCampaignNotDraft, "campaign must be in draft status to schedule")
	}

	// Resolve segment and create recipients.
	count, err := s.resolveAndCreateRecipients(ctx, c)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, apperrors.New(apperrors.CodeCampaignNoRecipients, "segment resolved to zero recipients")
	}

	// Update campaign.
	c.Status = StatusScheduled
	c.ScheduledAt = &scheduledAt
	c.TotalRecipients = count
	if err := s.repo.UpdateCampaign(s.db, c); err != nil {
		return nil, err
	}

	return c, nil
}

// SendCampaign transitions a campaign to "sending" so the send worker
// picks it up. If recipients don't exist yet, resolves the segment first.
func (s *Service) SendCampaign(ctx context.Context, campaignID uuid.UUID) (*Campaign, error) {
	c, err := s.repo.GetCampaignByID(ctx, s.db, campaignID)
	if err != nil {
		return nil, err
	}
	if c.Status != StatusDraft && c.Status != StatusScheduled {
		return nil, apperrors.New(apperrors.CodeCampaignNotDraft, "campaign must be in draft or scheduled status to send")
	}

	// Resolve and create recipients if not already done.
	existing, err := s.repo.CountRecipientsByCampaign(ctx, s.db, campaignID)
	if err != nil {
		return nil, err
	}
	if existing == 0 {
		count, err := s.resolveAndCreateRecipients(ctx, c)
		if err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, apperrors.New(apperrors.CodeCampaignNoRecipients, "segment resolved to zero recipients")
		}
		c.TotalRecipients = count
	}

	c.Status = StatusSending
	if err := s.repo.UpdateCampaign(s.db, c); err != nil {
		return nil, err
	}

	return c, nil
}

// PauseCampaign transitions a sending campaign to "paused".
func (s *Service) PauseCampaign(ctx context.Context, campaignID uuid.UUID) (*Campaign, error) {
	c, err := s.repo.GetCampaignByID(ctx, s.db, campaignID)
	if err != nil {
		return nil, err
	}
	if c.Status != StatusSending {
		return nil, apperrors.New(apperrors.CodeCampaignNotSending, "campaign must be in sending status to pause")
	}

	c.Status = StatusPaused
	if err := s.repo.UpdateCampaign(s.db, c); err != nil {
		return nil, err
	}
	return c, nil
}

// ResumeCampaign transitions a paused campaign back to "sending".
func (s *Service) ResumeCampaign(ctx context.Context, campaignID uuid.UUID) (*Campaign, error) {
	c, err := s.repo.GetCampaignByID(ctx, s.db, campaignID)
	if err != nil {
		return nil, err
	}
	if c.Status != StatusPaused {
		return nil, apperrors.New(apperrors.CodeCampaignNotPaused, "campaign must be in paused status to resume")
	}

	c.Status = StatusSending
	if err := s.repo.UpdateCampaign(s.db, c); err != nil {
		return nil, err
	}
	return c, nil
}

// DeleteCampaign removes a draft campaign.
func (s *Service) DeleteCampaign(ctx context.Context, campaignID uuid.UUID) error {
	c, err := s.repo.GetCampaignByID(ctx, s.db, campaignID)
	if err != nil {
		return err
	}
	if c.Status != StatusDraft {
		return apperrors.New(apperrors.CodeCampaignNotDraft, "only draft campaigns can be deleted")
	}
	return s.repo.DeleteCampaign(s.db, campaignID)
}

// --- Segments ---

// CreateSegment validates and persists a new customer segment.
func (s *Service) CreateSegment(ctx context.Context, seg *CustomerSegment) error {
	if seg.Name == "" {
		return apperrors.ValidationFailed("name", "name is required")
	}
	// Validate rules JSON is parseable.
	var rules []SegmentRule
	if err := parseRulesJSON(seg.Rules, &rules); err != nil {
		return apperrors.New(apperrors.CodeSegmentInvalidRules, fmt.Sprintf("invalid rules: %v", err))
	}
	return s.repo.CreateSegment(s.db, seg)
}

// ListSegments returns all segments for a store with live member counts.
// Each segment's cached MemberCount is replaced with a freshly-resolved
// count from the segment engine so the admin UI never shows stale numbers
// when customer tiers shift between campaign sends.
func (s *Service) ListSegments(ctx context.Context, storeID uuid.UUID) ([]CustomerSegment, error) {
	segments, err := s.repo.ListSegmentsByStore(ctx, s.db, storeID)
	if err != nil {
		return nil, err
	}
	for i := range segments {
		emails, resolveErr := s.segmentEngine.ResolveEmails(
			ctx, segments[i].TenantID, segments[i].StoreID, segments[i].Rules,
		)
		if resolveErr != nil {
			// Resolution failure (e.g. malformed legacy rules) shouldn't
			// hide the segment — keep the cached count and move on.
			continue
		}
		segments[i].MemberCount = len(emails)
	}
	return segments, nil
}

// GetSegment returns a single segment by ID.
func (s *Service) GetSegment(ctx context.Context, segmentID uuid.UUID) (*CustomerSegment, error) {
	return s.repo.GetSegmentByID(ctx, s.db, segmentID)
}

// UpdateSegment validates and persists changes to a segment's name,
// description, and rules.
func (s *Service) UpdateSegment(ctx context.Context, seg *CustomerSegment) error {
	if seg.Name == "" {
		return apperrors.ValidationFailed("name", "name is required")
	}
	var rules []SegmentRule
	if err := parseRulesJSON(seg.Rules, &rules); err != nil {
		return apperrors.New(apperrors.CodeSegmentInvalidRules, fmt.Sprintf("invalid rules: %v", err))
	}
	return s.repo.UpdateSegment(s.db, seg)
}

// DeleteSegment removes a segment by ID.
func (s *Service) DeleteSegment(ctx context.Context, segmentID uuid.UUID) error {
	return s.repo.DeleteSegment(s.db, segmentID)
}

// PreviewSegment resolves segment rules and returns the count and sample
// of matching emails (up to 10).
func (s *Service) PreviewSegment(ctx context.Context, tenantID, storeID uuid.UUID, rulesJSON []byte) (int, []string, error) {
	emails, err := s.segmentEngine.ResolveEmails(ctx, tenantID, storeID, rulesJSON)
	if err != nil {
		return 0, nil, err
	}
	sample := emails
	if len(sample) > 10 {
		sample = sample[:10]
	}
	return len(emails), sample, nil
}

// --- helpers ---

func (s *Service) resolveAndCreateRecipients(ctx context.Context, c *Campaign) (int, error) {
	// SegmentID == nil means "all customers" in the admin wizard — the
	// merchant left the segment picker on the default "All customers"
	// option. We synthesize an implicit [{"type":"all"}] rule set rather
	// than requiring the merchant to pre-create an "All" segment row.
	var rulesJSON []byte
	if c.SegmentID == nil {
		rulesJSON = []byte(`[{"type":"all"}]`)
	} else {
		seg, err := s.repo.GetSegmentByID(ctx, s.db, *c.SegmentID)
		if err != nil {
			return 0, err
		}
		rulesJSON = seg.Rules
	}

	emails, err := s.segmentEngine.ResolveEmails(ctx, c.TenantID, c.StoreID, rulesJSON)
	if err != nil {
		return 0, fmt.Errorf("campaign: resolve segment: %w", err)
	}
	if len(emails) == 0 {
		return 0, nil
	}

	// Build recipients.
	recipients := make([]CampaignRecipient, len(emails))
	for i, email := range emails {
		recipients[i] = CampaignRecipient{
			TenantID:      c.TenantID,
			CampaignID:    c.ID,
			CustomerEmail: email,
			Status:        RecipientPending,
		}
	}

	if err := s.repo.CreateRecipientsInBatches(s.db, recipients, SendBatchSize); err != nil {
		return 0, fmt.Errorf("campaign: create recipients: %w", err)
	}

	// Update segment member_count.
	_ = s.repo.UpdateSegmentMemberCount(s.db, *c.SegmentID, len(emails))

	return len(emails), nil
}

func parseRulesJSON(data []byte, out *[]SegmentRule) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}
