// services/marketplace-api/internal/stores/middleware.go
package stores

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// MiddlewareConfig groups the knobs for StoreMiddleware so production and
// test wiring can agree without a long parameter list.
type MiddlewareConfig struct {
	Repo      Repository
	Client    Client
	Logger    *slog.Logger
	Flight    *singleflight.Group // shared across requests to coalesce refreshes
	FreshTTL  time.Duration       // default 5 * time.Minute
	StaleCeil time.Duration       // default 24 * time.Hour
	TenantKey string              // gin context key that holds tenant id; default "tenant_id"
}

// StoreMiddleware enforces store-ownership and populates c.Set("store", ...).
// Implements spec §14.7.
func StoreMiddleware(cfg MiddlewareConfig) gin.HandlerFunc {
	if cfg.FreshTTL == 0 {
		cfg.FreshTTL = 5 * time.Minute
	}
	if cfg.StaleCeil == 0 {
		cfg.StaleCeil = 24 * time.Hour
	}
	if cfg.TenantKey == "" {
		cfg.TenantKey = "tenant_id"
	}
	return func(c *gin.Context) {
		storeID := c.Param("storeId")
		tenantID, _ := c.Get(cfg.TenantKey)
		tid, _ := tenantID.(string)
		if storeID == "" || tid == "" {
			respondNotFound(c)
			return
		}

		cached, cacheErr := cfg.Repo.GetByIDForTenant(c.Request.Context(), storeID, tid)
		fresh := cacheErr == nil && !IsStale(cached, cfg.FreshTTL)
		if fresh {
			c.Set("store", cached)
			c.Next()
			return
		}

		result, refreshErr, _ := cfg.Flight.Do("store:"+storeID, func() (interface{}, error) {
			return refresh(c.Request.Context(), cfg, storeID, tid)
		})

		switch {
		case refreshErr == nil && result != nil:
			c.Set("store", result.(*Store))
			c.Next()
		case cacheErr == nil && cached != nil && time.Since(cached.SyncedAt) < cfg.StaleCeil:
			if cfg.Logger != nil {
				cfg.Logger.Warn("serving stale store projection",
					"store_id", storeID,
					"synced_at", cached.SyncedAt,
					"refresh_err", refreshErr)
			}
			c.Set("store", cached)
			c.Set("store_stale", true)
			c.Next()
		default:
			respondNotFound(c)
		}
	}
}

func refresh(ctx context.Context, cfg MiddlewareConfig, storeID, tenantID string) (*Store, error) {
	fresh, err := cfg.Client.GetStore(ctx, tenantID, storeID)
	if err != nil {
		return nil, err
	}
	if fresh == nil || fresh.TenantID != tenantID {
		return nil, ErrNotFound
	}
	fresh.SyncedAt = time.Now()
	if err := cfg.Repo.Upsert(ctx, fresh); err != nil {
		return nil, err
	}
	return fresh, nil
}

func respondNotFound(c *gin.Context) {
	c.AbortWithStatusJSON(404, map[string]any{
		"error":   string(apperrors.CodeNotFound),
		"message": "store not found",
	})
}

// ensure errors.Is line wraps are not dead code (reserved for M5 handler use)
var _ = errors.Is
