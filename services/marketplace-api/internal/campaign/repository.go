package campaign

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Repository is the data-access surface for campaigns, segments, and
// recipients. Read methods take context + db. Mutating methods take
// explicit *gorm.DB so callers can pass their own transaction.
type Repository interface {
	// --- Segments ---
	CreateSegment(tx *gorm.DB, s *CustomerSegment) error
	ListSegmentsByStore(ctx context.Context, db *gorm.DB, storeID uuid.UUID) ([]CustomerSegment, error)
	GetSegmentByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*CustomerSegment, error)
	DeleteSegment(tx *gorm.DB, id uuid.UUID) error
	UpdateSegmentMemberCount(tx *gorm.DB, id uuid.UUID, count int) error

	// --- Campaigns ---
	CreateCampaign(tx *gorm.DB, c *Campaign) error
	GetCampaignByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*Campaign, error)
	ListCampaignsByStore(ctx context.Context, db *gorm.DB, storeID uuid.UUID, status string, page, pageSize int) ([]Campaign, int64, error)
	UpdateCampaign(tx *gorm.DB, c *Campaign) error
	UpdateCampaignStatus(tx *gorm.DB, id uuid.UUID, status string) error
	DeleteCampaign(tx *gorm.DB, id uuid.UUID) error

	// UpdateHeartbeat sets heartbeat_at = now() for the given campaign.
	UpdateHeartbeat(tx *gorm.DB, id uuid.UUID) error

	// IncrementAnalytics atomically increments a single analytics counter
	// column. column must be one of the Analytics* typed constants.
	// Uses raw SQL UPDATE ... SET col = col + 1.
	IncrementAnalytics(tx *gorm.DB, id uuid.UUID, column string) error

	// SetSentAt marks campaign as sent with a timestamp.
	SetSentAt(tx *gorm.DB, id uuid.UUID, sentAt time.Time) error

	// FindStuckCampaigns returns campaigns with status='sending' whose
	// heartbeat is stale. Uses FOR UPDATE SKIP LOCKED inside a transaction.
	FindStuckCampaigns(ctx context.Context, db *gorm.DB, staleDuration time.Duration) ([]Campaign, error)

	// FindSendableCampaigns returns campaigns in "sending" status across
	// all stores (the worker is not store-scoped).
	FindSendableCampaigns(ctx context.Context, db *gorm.DB) ([]Campaign, error)

	// FindScheduledReady returns campaigns in "scheduled" status whose
	// scheduled_at is in the past.
	FindScheduledReady(ctx context.Context, db *gorm.DB) ([]Campaign, error)

	// FindActiveStorefrontPromotion returns the highest-priority campaign
	// with show_on_storefront=true and a non-terminal status for a store.
	FindActiveStorefrontPromotion(ctx context.Context, db *gorm.DB, storeID uuid.UUID) (*Campaign, error)

	// --- Recipients ---
	CreateRecipientsInBatches(tx *gorm.DB, recipients []CampaignRecipient, batchSize int) error
	ListRecipientsByCampaign(ctx context.Context, db *gorm.DB, campaignID uuid.UUID, status string, page, pageSize int) ([]CampaignRecipient, int64, error)
	GetPendingRecipients(ctx context.Context, db *gorm.DB, campaignID uuid.UUID, limit int) ([]CampaignRecipient, error)
	UpdateRecipientStatus(tx *gorm.DB, id uuid.UUID, status string) error
	CountRecipientsByCampaign(ctx context.Context, db *gorm.DB, campaignID uuid.UUID) (int64, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository constructs a campaign repository.
func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

// --- Segments ---

func (r *gormRepository) CreateSegment(tx *gorm.DB, s *CustomerSegment) error {
	return tx.Create(s).Error
}

func (r *gormRepository) ListSegmentsByStore(ctx context.Context, db *gorm.DB, storeID uuid.UUID) ([]CustomerSegment, error) {
	var segments []CustomerSegment
	err := db.WithContext(ctx).Where("store_id = ?", storeID).Order("created_at DESC").Find(&segments).Error
	return segments, err
}

func (r *gormRepository) GetSegmentByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*CustomerSegment, error) {
	var seg CustomerSegment
	err := db.WithContext(ctx).Where("id = ?", id).First(&seg).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.New(apperrors.CodeSegmentNotFound, "segment not found")
		}
		return nil, err
	}
	return &seg, nil
}

func (r *gormRepository) DeleteSegment(tx *gorm.DB, id uuid.UUID) error {
	res := tx.Where("id = ?", id).Delete(&CustomerSegment{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return apperrors.New(apperrors.CodeSegmentNotFound, "segment not found")
	}
	return nil
}

func (r *gormRepository) UpdateSegmentMemberCount(tx *gorm.DB, id uuid.UUID, count int) error {
	return tx.Model(&CustomerSegment{}).Where("id = ?", id).Update("member_count", count).Error
}

// --- Campaigns ---

func (r *gormRepository) CreateCampaign(tx *gorm.DB, c *Campaign) error {
	return tx.Create(c).Error
}

func (r *gormRepository) GetCampaignByID(ctx context.Context, db *gorm.DB, id uuid.UUID) (*Campaign, error) {
	var c Campaign
	err := db.WithContext(ctx).Where("id = ?", id).First(&c).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.New(apperrors.CodeCampaignNotFound, "campaign not found")
		}
		return nil, err
	}
	return &c, nil
}

func (r *gormRepository) ListCampaignsByStore(ctx context.Context, db *gorm.DB, storeID uuid.UUID, status string, page, pageSize int) ([]Campaign, int64, error) {
	var campaigns []Campaign
	var total int64
	q := db.WithContext(ctx).Model(&Campaign{}).Where("store_id = ?", storeID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	err := q.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&campaigns).Error
	return campaigns, total, err
}

func (r *gormRepository) UpdateCampaign(tx *gorm.DB, c *Campaign) error {
	return tx.Save(c).Error
}

func (r *gormRepository) UpdateCampaignStatus(tx *gorm.DB, id uuid.UUID, status string) error {
	res := tx.Model(&Campaign{}).Where("id = ?", id).Updates(map[string]any{
		"status":     status,
		"updated_at": time.Now(),
	})
	return res.Error
}

func (r *gormRepository) DeleteCampaign(tx *gorm.DB, id uuid.UUID) error {
	res := tx.Where("id = ?", id).Delete(&Campaign{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return apperrors.New(apperrors.CodeCampaignNotFound, "campaign not found")
	}
	return nil
}

func (r *gormRepository) UpdateHeartbeat(tx *gorm.DB, id uuid.UUID) error {
	res := tx.Model(&Campaign{}).Where("id = ?", id).
		Update("heartbeat_at", gorm.Expr("now()"))
	if res.Error != nil {
		return fmt.Errorf("campaign: update heartbeat: %w", res.Error)
	}
	return nil
}

func (r *gormRepository) IncrementAnalytics(tx *gorm.DB, id uuid.UUID, column string) error {
	// Allowlist of valid columns to prevent SQL injection.
	allowed := map[string]bool{
		AnalyticsDelivered:    true,
		AnalyticsOpened:       true,
		AnalyticsClicked:      true,
		AnalyticsConverted:    true,
		AnalyticsUnsubscribed: true,
		AnalyticsFailed:       true,
	}
	if !allowed[column] {
		return fmt.Errorf("campaign: invalid analytics column %q", column)
	}
	res := tx.Model(&Campaign{}).Where("id = ?", id).
		Update(column, gorm.Expr(column+" + 1"))
	return res.Error
}

func (r *gormRepository) SetSentAt(tx *gorm.DB, id uuid.UUID, sentAt time.Time) error {
	return tx.Model(&Campaign{}).Where("id = ?", id).Updates(map[string]any{
		"sent_at":    sentAt,
		"updated_at": time.Now(),
	}).Error
}

func (r *gormRepository) FindStuckCampaigns(ctx context.Context, db *gorm.DB, staleDuration time.Duration) ([]Campaign, error) {
	var campaigns []Campaign
	cutoff := time.Now().Add(-staleDuration)
	// MEDIUM FIX 5: wrap in transaction for FOR UPDATE SKIP LOCKED to work.
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Raw(
			"SELECT * FROM campaigns WHERE status = ? AND (heartbeat_at IS NULL OR heartbeat_at < ?) FOR UPDATE SKIP LOCKED",
			StatusSending, cutoff,
		).Scan(&campaigns).Error
	})
	return campaigns, err
}

func (r *gormRepository) FindSendableCampaigns(ctx context.Context, db *gorm.DB) ([]Campaign, error) {
	var campaigns []Campaign
	err := db.WithContext(ctx).
		Where("status = ?", StatusSending).
		Order("updated_at ASC").
		Limit(10).
		Find(&campaigns).Error
	return campaigns, err
}

func (r *gormRepository) FindScheduledReady(ctx context.Context, db *gorm.DB) ([]Campaign, error) {
	var campaigns []Campaign
	err := db.WithContext(ctx).
		Where("status = ? AND scheduled_at <= ?", StatusScheduled, time.Now()).
		Find(&campaigns).Error
	return campaigns, err
}

func (r *gormRepository) FindActiveStorefrontPromotion(ctx context.Context, db *gorm.DB, storeID uuid.UUID) (*Campaign, error) {
	var c Campaign
	err := db.WithContext(ctx).
		Where("store_id = ? AND show_on_storefront = true AND status NOT IN ?",
			storeID, []string{StatusCancelled}).
		Order("storefront_priority DESC, created_at DESC").
		First(&c).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

// --- Recipients ---

func (r *gormRepository) CreateRecipientsInBatches(tx *gorm.DB, recipients []CampaignRecipient, batchSize int) error {
	if len(recipients) == 0 {
		return nil
	}
	return tx.CreateInBatches(&recipients, batchSize).Error
}

func (r *gormRepository) ListRecipientsByCampaign(ctx context.Context, db *gorm.DB, campaignID uuid.UUID, status string, page, pageSize int) ([]CampaignRecipient, int64, error) {
	var recipients []CampaignRecipient
	var total int64
	q := db.WithContext(ctx).Model(&CampaignRecipient{}).Where("campaign_id = ?", campaignID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	err := q.Order("created_at ASC").Offset(offset).Limit(pageSize).Find(&recipients).Error
	return recipients, total, err
}

func (r *gormRepository) GetPendingRecipients(ctx context.Context, db *gorm.DB, campaignID uuid.UUID, limit int) ([]CampaignRecipient, error) {
	var recipients []CampaignRecipient
	err := db.WithContext(ctx).
		Where("campaign_id = ? AND status = ?", campaignID, RecipientPending).
		Order("created_at ASC").
		Limit(limit).
		Find(&recipients).Error
	return recipients, err
}

func (r *gormRepository) UpdateRecipientStatus(tx *gorm.DB, id uuid.UUID, status string) error {
	updates := map[string]any{"status": status}
	switch status {
	case RecipientSent:
		now := time.Now()
		updates["sent_at"] = now
	case RecipientOpened:
		now := time.Now()
		updates["opened_at"] = now
	case RecipientClicked:
		now := time.Now()
		updates["clicked_at"] = now
	}
	return tx.Model(&CampaignRecipient{}).Where("id = ?", id).Updates(updates).Error
}

func (r *gormRepository) CountRecipientsByCampaign(ctx context.Context, db *gorm.DB, campaignID uuid.UUID) (int64, error) {
	var count int64
	err := db.WithContext(ctx).Model(&CampaignRecipient{}).Where("campaign_id = ?", campaignID).Count(&count).Error
	return count, err
}
