// Package admin — subscription_complete_action.go: GET /subscription/complete-action
// redirects a merchant to the Stripe-hosted invoice payment page. The URL is
// populated by the SCA webhook (§4.7) and cleared once invoice.paid fires.
package admin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// subscriptionLoader abstracts the DB fetch so unit tests can inject a fake
// without standing up a database. The production implementation is dbSubLoader.
type subscriptionLoader interface {
	LoadForTenant(ctx context.Context, tenantID, storeID uuid.UUID) (*subscription.StoreSubscription, error)
}

// SubscriptionLoaderForTest is an exported alias kept for test readability;
// the interface is otherwise unexported to minimise the package surface.
type SubscriptionLoaderForTest = subscriptionLoader

// dbSubLoader is the production subscriptionLoader backed by GORM.
type dbSubLoader struct{ db *gorm.DB }

func (l *dbSubLoader) LoadForTenant(ctx context.Context, tenantID, storeID uuid.UUID) (*subscription.StoreSubscription, error) {
	var sub subscription.StoreSubscription
	err := l.db.WithContext(ctx).
		Where("tenant_id = ? AND store_id = ?", tenantID, storeID).
		First(&sub).Error
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

// CompleteActionHandler redirects the merchant to the currently stored
// Stripe hosted_invoice_url. The URL is populated by the SCA webhook (§4.7).
// Mounted on the subscription group and allowlisted in RequireActive — an
// expired merchant can still reach this route to pay and recover.
type CompleteActionHandler struct {
	loader subscriptionLoader
	logger *slog.Logger
}

// NewCompleteActionHandler constructs a CompleteActionHandler backed by the
// given GORM DB. Pass a non-nil logger or slog.Default() is used.
func NewCompleteActionHandler(db *gorm.DB, logger *slog.Logger) *CompleteActionHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &CompleteActionHandler{
		loader: &dbSubLoader{db: db},
		logger: logger,
	}
}

// NewCompleteActionHandlerWithLoader constructs a CompleteActionHandler with a
// caller-supplied loader. Used by unit tests to inject a fake without a DB.
func NewCompleteActionHandlerWithLoader(loader SubscriptionLoaderForTest, logger *slog.Logger) *CompleteActionHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &CompleteActionHandler{loader: loader, logger: logger}
}

// Redirect handles GET /admin/stores/:storeId/subscription/complete-action.
// 302s to hosted_invoice_url on the StoreSubscription row; 404s when there is
// no URL (no pending SCA action).
func (h *CompleteActionHandler) Redirect(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return
	}

	sub, err := h.loader.LoadForTenant(c.Request.Context(), tenantID, storeID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "subscription_not_found"})
			return
		}
		h.logger.Error("complete-action: load sub failed", "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}

	if sub.HostedInvoiceURL == nil || *sub.HostedInvoiceURL == "" {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "no_pending_action"})
		return
	}

	c.Redirect(http.StatusFound, *sub.HostedInvoiceURL)
}
