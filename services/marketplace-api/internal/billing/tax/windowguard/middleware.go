// Package windowguard exposes the publish-only Gin middleware that enforces
// the §5.2 14-day tax-validation window. It does NOT block read or admin
// routes — that's the readonly.RequireActive story (P3). This middleware is
// mounted on storefront-publish routes only.
package windowguard

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/tax"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// SubscriptionLoader is the minimal lookup contract this middleware needs.
// subscription.Repository satisfies it; tests pass an in-memory fake.
type SubscriptionLoader interface {
	GetByStoreID(ctx context.Context, db *gorm.DB, tenantID, storeID uuid.UUID) (*subscription.StoreSubscription, error)
}

const (
	// StandardWindow — §5.2: merchants have 14 days from signup to validate
	// their tax ID before storefront-publish is blocked.
	StandardWindow = 14 * 24 * time.Hour
	// FastPathWindow — §5.1.1: CSM-approved migration fast-path shrinks the
	// window to 48 hours, anchored to the original signup timestamp.
	FastPathWindow = 48 * time.Hour
)

// Config wires the middleware. Repo and Clock are required; NowFunc is
// injectable for tests.
type Config struct {
	DB      *gorm.DB
	Repo    SubscriptionLoader
	Clock   *tax.ClockPauseTracker
	NowFunc func() time.Time
}

// RequirePublishable returns a Gin middleware that aborts with HTTP 403 when
// the window has elapsed and the merchant is unvalidated and the clock is not
// currently paused. Validated merchants and merchants whose clock is paused
// pass through.
func RequirePublishable(cfg Config) gin.HandlerFunc {
	if cfg.NowFunc == nil {
		cfg.NowFunc = func() time.Time { return time.Now().UTC() }
	}
	return func(c *gin.Context) {
		tid, _ := c.Get("tenant_id")
		tenantStr, _ := tid.(string)
		tenantUUID, err := uuid.Parse(tenantStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing_tenant"})
			return
		}
		storeUUID, err := uuid.Parse(c.Param("storeId"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_store_id"})
			return
		}

		sub, err := cfg.Repo.GetByStoreID(c.Request.Context(), cfg.DB, tenantUUID, storeUUID)
		if err != nil {
			if isNotFound(err) {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "subscription_not_found"})
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "subscription_lookup_failed"})
			return
		}

		if sub.TaxIDValidated {
			c.Next()
			return
		}

		country := ""
		if sub.TaxIDCountry != nil {
			country = *sub.TaxIDCountry
		}
		if cfg.Clock != nil && country != "" {
			paused, err := cfg.Clock.IsPaused(c.Request.Context(), sub.StoreID, country)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "clock_check_failed"})
				return
			}
			if paused {
				c.Next()
				return
			}
		}

		window := StandardWindow
		if sub.TaxIDWindowShortenedAt != nil {
			window = FastPathWindow
		}

		if cfg.NowFunc().Sub(sub.CreatedAt) > window {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":  "tax_validation_window_expired",
				"window": window.String(),
			})
			return
		}
		c.Next()
	}
}

func isNotFound(err error) bool {
	var ae *apperrors.Error
	if errors.As(err, &ae) {
		return ae.Code == apperrors.CodeNotFound
	}
	return false
}
