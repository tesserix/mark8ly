package campaign

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// SendBatchSize is the number of recipients per dispatch batch.
	SendBatchSize = 500
	// SendBatchDelay is the inter-batch delay.
	SendBatchDelay = 1 * time.Second
	// HeartbeatInterval is the heartbeat update interval.
	HeartbeatInterval = 5 * time.Second
	// StaleDuration is the threshold for stuck campaign detection.
	StaleDuration = 15 * time.Minute
	// PollInterval is the worker poll interval for new sendable campaigns.
	PollInterval = 5 * time.Second
)

// ThemeLoader resolves a CampaignTheme for a given store at send time.
// Kept as a narrow interface so we don't couple the campaign package to
// branding or stores internals. The send worker calls it ONCE per
// campaign dispatch so the merchant's branding is fetched in a single
// query, not once per recipient.
type ThemeLoader interface {
	LoadTheme(ctx context.Context, storeID uuid.UUID) (CampaignTheme, error)
}

// SendWorkerConfig bundles send worker dependencies.
type SendWorkerConfig struct {
	DB          *gorm.DB
	Repo        Repository
	Dispatcher  Dispatcher
	ThemeLoader ThemeLoader // optional — nil falls back to default theme
	Logger      *slog.Logger
}

// SendWorker polls for campaigns in "sending" status and dispatches
// recipients in batches.
type SendWorker struct {
	db          *gorm.DB
	repo        Repository
	dispatcher  Dispatcher
	themeLoader ThemeLoader
	logger      *slog.Logger
}

// NewSendWorker constructs a send worker.
func NewSendWorker(cfg SendWorkerConfig) *SendWorker {
	return &SendWorker{
		db:          cfg.DB,
		repo:        cfg.Repo,
		dispatcher:  cfg.Dispatcher,
		themeLoader: cfg.ThemeLoader,
		logger:      cfg.Logger,
	}
}

// RecoverStuckCampaigns finds campaigns with status='sending' and stale
// heartbeat, resets them to 'paused'. Same pattern as csvjob.RecoverOrphanedJobs.
func RecoverStuckCampaigns(ctx context.Context, repo Repository, db *gorm.DB, staleDuration time.Duration, logger *slog.Logger) error {
	campaigns, err := repo.FindStuckCampaigns(ctx, db, staleDuration)
	if err != nil {
		return fmt.Errorf("campaign: recover stuck: %w", err)
	}
	for _, c := range campaigns {
		logger.Info("campaign: recovering stuck campaign", "campaign_id", c.ID, "heartbeat_at", c.HeartbeatAt)
		if err := repo.UpdateCampaignStatus(db, c.ID, StatusPaused); err != nil {
			logger.Error("campaign: recover stuck", "campaign_id", c.ID, "err", err)
		}
	}
	return nil
}

// Run starts the send worker polling loop. Blocks until ctx is cancelled.
func (w *SendWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.pollAndDispatch(ctx)
		}
	}
}

// pollAndDispatch finds sendable and scheduled-ready campaigns and
// processes them. Uses FindSendableCampaigns (HIGH FIX 2) instead of
// ListCampaignsByStore with uuid.Nil.
func (w *SendWorker) pollAndDispatch(ctx context.Context) {
	// Activate scheduled campaigns whose scheduled_at has passed.
	ready, err := w.repo.FindScheduledReady(ctx, w.db)
	if err != nil {
		w.logger.Error("campaign: find scheduled ready", "err", err)
	}
	for _, c := range ready {
		w.logger.Info("campaign: activating scheduled campaign", "campaign_id", c.ID)
		if err := w.repo.UpdateCampaignStatus(w.db, c.ID, StatusSending); err != nil {
			w.logger.Error("campaign: activate scheduled", "campaign_id", c.ID, "err", err)
		}
	}

	// Find campaigns in 'sending' status (cross-store).
	campaigns, err := w.repo.FindSendableCampaigns(ctx, w.db)
	if err != nil {
		w.logger.Error("campaign: find sendable", "err", err)
		return
	}
	for _, c := range campaigns {
		if err := w.dispatchCampaign(ctx, c); err != nil {
			w.logger.Error("campaign: dispatch error", "campaign_id", c.ID, "err", err)
		}
	}
}

func (w *SendWorker) dispatchCampaign(ctx context.Context, c Campaign) error {
	// Start heartbeat ticker.
	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	go w.heartbeatLoop(heartbeatCtx, c.ID)

	// Pull campaign-level fields once — they don't change per recipient.
	subject := ""
	if c.Subject != nil {
		subject = *c.Subject
	}
	body := ""
	if c.Content != nil {
		body = *c.Content
	}

	// Load the merchant's storefront theme once per campaign dispatch.
	// Render the HTML + text envelope once — emails to all recipients of
	// this campaign share the same body, so rendering per-recipient would
	// be pure waste.
	theme := CampaignTheme{}
	if w.themeLoader != nil {
		loaded, err := w.themeLoader.LoadTheme(ctx, c.StoreID)
		if err != nil {
			w.logger.Warn("campaign: theme load failed — using defaults",
				"campaign_id", c.ID, "store_id", c.StoreID, "err", err)
		} else {
			theme = loaded
		}
	}
	htmlBody, err := RenderCampaignHTML(subject, body, theme)
	if err != nil {
		return fmt.Errorf("render html: %w", err)
	}
	textBody, err := RenderCampaignText(subject, body, theme)
	if err != nil {
		return fmt.Errorf("render text: %w", err)
	}

	for {
		// Fetch next batch of pending recipients.
		recipients, err := w.repo.GetPendingRecipients(ctx, w.db, c.ID, SendBatchSize)
		if err != nil {
			return fmt.Errorf("fetch pending: %w", err)
		}
		if len(recipients) == 0 {
			break // All dispatched.
		}

		for _, r := range recipients {
			outbound := OutboundEmail{
				Recipient: r.CustomerEmail,
				Subject:   subject,
				HTMLBody:  htmlBody,
				TextBody:  textBody,
			}
			if err := w.dispatcher.Send(ctx, outbound); err != nil {
				w.logger.Error("campaign: dispatch failed",
					"campaign_id", c.ID,
					"email", r.CustomerEmail,
					"err", err)
				_ = w.repo.IncrementAnalytics(w.db, c.ID, AnalyticsFailed)
				continue
			}

			if err := w.repo.UpdateRecipientStatus(w.db, r.ID, RecipientSent); err != nil {
				w.logger.Error("campaign: update recipient", "id", r.ID, "err", err)
				_ = w.repo.IncrementAnalytics(w.db, c.ID, AnalyticsFailed)
				continue
			}
			_ = w.repo.IncrementAnalytics(w.db, c.ID, AnalyticsDelivered)
		}

		// Check for pause/cancel.
		refreshed, err := w.repo.GetCampaignByID(ctx, w.db, c.ID)
		if err != nil {
			return err
		}
		if refreshed.Status == StatusPaused || refreshed.Status == StatusCancelled {
			w.logger.Info("campaign: stopped by user", "campaign_id", c.ID, "status", refreshed.Status)
			return nil
		}

		// Inter-batch delay.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(SendBatchDelay):
		}
	}

	// Mark campaign as sent.
	now := time.Now()
	if err := w.repo.SetSentAt(w.db, c.ID, now); err != nil {
		return err
	}
	return w.repo.UpdateCampaignStatus(w.db, c.ID, StatusSent)
}

func (w *SendWorker) heartbeatLoop(ctx context.Context, campaignID uuid.UUID) {
	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.repo.UpdateHeartbeat(w.db, campaignID); err != nil {
				w.logger.Error("campaign: heartbeat", "campaign_id", campaignID, "err", err)
			}
		}
	}
}
