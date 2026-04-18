package readonly

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// StatusLoaderConfig wires a GORM-backed subscription lookup into the Gin chain.
type StatusLoaderConfig struct {
	DB         *gorm.DB
	Repo       subscription.Repository
	Logger     *slog.Logger
	ContextKey string // default "subscription_status"
}

// LoadStatus reads tenant_id + storeId from the context/params, looks up the
// subscription, and sets the status (plus plan + app-addon flag) on the Gin
// context. A missing subscription is silent — RequireActive treats "no status
// key" as "no decision" and falls through.
func LoadStatus(cfg StatusLoaderConfig) gin.HandlerFunc {
	if cfg.ContextKey == "" {
		cfg.ContextKey = "subscription_status"
	}
	return func(c *gin.Context) {
		tid, _ := c.Get("tenant_id")
		tenantStr, _ := tid.(string)
		tenantUUID, err := uuid.Parse(tenantStr)
		if err != nil {
			c.Next()
			return
		}
		storeUUID, err := uuid.Parse(c.Param("storeId"))
		if err != nil {
			c.Next()
			return
		}

		sub, err := cfg.Repo.GetByStoreID(c.Request.Context(), cfg.DB, tenantUUID, storeUUID)
		if err != nil {
			if !isNotFound(err) {
				if cfg.Logger != nil {
					cfg.Logger.Error("readonly.LoadStatus: lookup failed",
						"tenant_id", tenantUUID, "store_id", storeUUID, "err", err)
				}
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "subscription_lookup_failed",
				})
				return
			}
			c.Next()
			return
		}
		c.Set(cfg.ContextKey, sub.Status)
		c.Set("subscription_plan", sub.Plan)
		c.Set("subscription_has_app_addon", sub.HasWhiteLabelAppAddOn)
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
