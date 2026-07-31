package authz

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// LazyConfig configures a LazyClient.
type LazyConfig struct {
	APIURL  string
	StoreID string // optional — discovered by name when empty
	ModelID string
	// StoreName is the store to discover when StoreID is empty.
	// Defaults to FGAStoreName.
	StoreName string
	Logger    *slog.Logger
}

// LazyClient resolves the OpenFGA store on first use instead of at startup.
//
// Startup-time discovery made auth-bff's authorization permanently dead if
// openfga was not yet listening when the process booted — a pod-ordering
// race no restart of openfga could heal. Resolving per call means the
// outage window returns 503 (callers already retry) and the first request
// after openfga recovers caches the real client.
type LazyClient struct {
	cfg LazyConfig

	mu       sync.Mutex
	resolved Client
}

// NewLazy constructs a LazyClient. It performs no I/O.
func NewLazy(cfg LazyConfig) *LazyClient {
	if cfg.StoreName == "" {
		cfg.StoreName = FGAStoreName
	}
	return &LazyClient{cfg: cfg}
}

// resolve returns the cached client, constructing it on first success.
func (l *LazyClient) resolve(ctx context.Context) (Client, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.resolved != nil {
		return l.resolved, nil
	}

	storeID := l.cfg.StoreID
	if storeID == "" {
		discovered, err := DiscoverStoreID(ctx, l.cfg.APIURL, l.cfg.StoreName)
		if err != nil {
			return nil, fmt.Errorf("authz: discover store: %w", err)
		}
		if discovered == "" {
			return nil, fmt.Errorf("authz: no store named %q", l.cfg.StoreName)
		}
		storeID = discovered
	}

	c, err := New(Config{
		APIURL:  l.cfg.APIURL,
		StoreID: storeID,
		ModelID: l.cfg.ModelID,
	})
	if err != nil {
		return nil, err
	}

	if l.cfg.Logger != nil {
		l.cfg.Logger.Info("authz: openfga store resolved", "store_id", storeID)
	}
	l.resolved = c
	return c, nil
}

// CheckMembership resolves the store if needed, then delegates.
func (l *LazyClient) CheckMembership(ctx context.Context, userID, tenantID string) (bool, error) {
	c, err := l.resolve(ctx)
	if err != nil {
		return false, err
	}
	return c.CheckMembership(ctx, userID, tenantID)
}
