// services/marketplace-api/internal/stores/middleware_test.go
package stores_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/stores"
)

const (
	testTenantID = "11111111-1111-1111-1111-111111111111"
	testStoreID  = "22222222-2222-2222-2222-222222222222"
)

func init() { gin.SetMode(gin.TestMode) }

// --- fakes ---

type fakeRepo struct {
	mu      sync.Mutex
	byKey   map[string]*stores.Store
	upserts int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byKey: map[string]*stores.Store{}}
}

func (r *fakeRepo) GetByIDForTenant(_ context.Context, storeID, tenantID string) (*stores.Store, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byKey[tenantID+"|"+storeID]
	if !ok {
		return nil, stores.ErrNotFound
	}
	cp := *s
	return &cp, nil
}

func (r *fakeRepo) GetBySlug(_ context.Context, slug string) (*stores.Store, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.byKey {
		if s.Slug == slug {
			cp := *s
			return &cp, nil
		}
	}
	return nil, stores.ErrNotFound
}

func (r *fakeRepo) ListForTenant(_ context.Context, tenantID string) ([]stores.Store, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []stores.Store
	for _, s := range r.byKey {
		if s.TenantID == tenantID {
			out = append(out, *s)
		}
	}
	return out, nil
}

func (r *fakeRepo) GetProductsWatermark(_ context.Context, _ string) (time.Time, error) {
	return time.Unix(0, 0), nil
}

func (r *fakeRepo) Upsert(_ context.Context, s *stores.Store) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *s
	r.byKey[s.TenantID+"|"+s.ID] = &cp
	r.upserts++
	return nil
}

func (r *fakeRepo) CountActiveOrSoftDeletedRestorable(_ context.Context, _ uuid.UUID) (int, error) {
	return 0, nil
}

func (r *fakeRepo) CountActiveOrSoftDeletedRestorableTx(_ context.Context, _ *gorm.DB, _ uuid.UUID) (int, error) {
	return 0, nil
}

func (r *fakeRepo) ListActiveOrSoftDeletedRestorable(_ context.Context, _ uuid.UUID) ([]stores.Store, error) {
	return nil, nil
}

func (r *fakeRepo) InFlightOrderCount(_ context.Context, _ uuid.UUID) (int, error) {
	return 0, nil
}

func (r *fakeRepo) SuspendActiveForTenant(_ context.Context, tenantID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, s := range r.byKey {
		if s.TenantID == tenantID && s.Status == stores.StatusActive {
			cp := *s
			cp.Status = stores.StatusSuspended
			r.byKey[k] = &cp
		}
	}
	return nil
}

func (r *fakeRepo) MarkStaleForTenant(_ context.Context, tenantID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, s := range r.byKey {
		if s.TenantID == tenantID {
			cp := *s
			// Mirrors the real repository's forceRefreshAge: stale enough
			// to fail IsStale against the default 5-minute FreshTTL, but
			// nowhere near the 24h StaleCeil, so a failed refresh still
			// falls into the fail-open stale-serve branch instead of 404ing.
			cp.SyncedAt = time.Now().Add(-10 * time.Minute)
			r.byKey[k] = &cp
		}
	}
	return nil
}

func (r *fakeRepo) preload(s *stores.Store) {
	_ = r.Upsert(context.Background(), s)
	r.mu.Lock()
	r.upserts = 0
	r.mu.Unlock()
}

func (r *fakeRepo) upsertCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.upserts
}

type fakeClient struct {
	mu     sync.Mutex
	calls  atomic.Int64
	result *stores.Store
	err    error
	delay  time.Duration
}

func (c *fakeClient) GetStore(_ context.Context, tenantID, storeID string) (*stores.Store, error) {
	c.calls.Add(1)
	if c.delay > 0 {
		time.Sleep(c.delay)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return nil, c.err
	}
	if c.result == nil {
		return nil, stores.ErrPlatformUnavailable
	}
	cp := *c.result
	return &cp, nil
}

func (c *fakeClient) GetStoreBySlug(_ context.Context, _ string) (*stores.Store, error) {
	return nil, stores.ErrPlatformUnavailable
}

// --- helpers ---

func newFixtureStore(syncedAt time.Time) *stores.Store {
	return &stores.Store{
		ID:           testStoreID,
		TenantID:     testTenantID,
		Slug:         "test-store",
		Name:         "Test Store",
		CountryCode:  "US",
		CurrencyCode: "USD",
		Timezone:     "UTC",
		Status:       stores.StatusActive,
		SyncedAt:     syncedAt,
	}
}

type probe struct {
	reached    bool
	store      *stores.Store
	storeStale bool
}

func buildRouter(cfg stores.MiddlewareConfig, p *probe) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", testTenantID)
	})
	r.GET("/stores/:storeId", stores.StoreMiddleware(cfg), func(c *gin.Context) {
		p.reached = true
		if v, ok := c.Get("store"); ok {
			p.store, _ = v.(*stores.Store)
		}
		if v, ok := c.Get("store_stale"); ok {
			p.storeStale, _ = v.(bool)
		}
		c.String(http.StatusOK, "ok")
	})
	return r
}

func doRequest(r *gin.Engine) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/stores/"+testStoreID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func baseCfg(repo stores.Repository, client stores.Client) stores.MiddlewareConfig {
	return stores.MiddlewareConfig{
		Repo:   repo,
		Client: client,
		Flight: &singleflight.Group{},
	}
}

// --- tests ---

func TestMiddleware_FreshCacheHit(t *testing.T) {
	repo := newFakeRepo()
	repo.preload(newFixtureStore(time.Now()))
	client := &fakeClient{}
	p := &probe{}
	r := buildRouter(baseCfg(repo, client), p)

	w := doRequest(r)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	if !p.reached {
		t.Fatal("handler not reached")
	}
	if p.store == nil || p.store.ID != testStoreID {
		t.Fatalf("store not set: %+v", p.store)
	}
	if client.calls.Load() != 0 {
		t.Fatalf("client calls: got %d want 0", client.calls.Load())
	}
}

func TestMiddleware_MissingCache_SuccessfulRefresh(t *testing.T) {
	repo := newFakeRepo()
	client := &fakeClient{result: newFixtureStore(time.Time{})}
	p := &probe{}
	r := buildRouter(baseCfg(repo, client), p)

	w := doRequest(r)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	if !p.reached || p.store == nil {
		t.Fatal("handler not reached / store not set")
	}
	if repo.upsertCount() != 1 {
		t.Fatalf("upserts: got %d want 1", repo.upsertCount())
	}
	if client.calls.Load() != 1 {
		t.Fatalf("client calls: got %d want 1", client.calls.Load())
	}
}

func TestMiddleware_StaleCache_SuccessfulRefresh(t *testing.T) {
	repo := newFakeRepo()
	repo.preload(newFixtureStore(time.Now().Add(-6 * time.Minute)))
	client := &fakeClient{result: newFixtureStore(time.Time{})}
	p := &probe{}
	r := buildRouter(baseCfg(repo, client), p)

	w := doRequest(r)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	if !p.reached {
		t.Fatal("handler not reached")
	}
	if repo.upsertCount() != 1 {
		t.Fatalf("upserts: got %d want 1", repo.upsertCount())
	}
	if client.calls.Load() != 1 {
		t.Fatalf("client calls: got %d want 1", client.calls.Load())
	}
	if p.storeStale {
		t.Fatal("store_stale should not be set on successful refresh")
	}
}

func TestMiddleware_StaleCache_PlatformOutage_ServesStale(t *testing.T) {
	repo := newFakeRepo()
	repo.preload(newFixtureStore(time.Now().Add(-6 * time.Minute)))
	client := &fakeClient{err: stores.ErrPlatformUnavailable}
	p := &probe{}
	r := buildRouter(baseCfg(repo, client), p)

	w := doRequest(r)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	if !p.reached {
		t.Fatal("handler not reached")
	}
	if p.store == nil {
		t.Fatal("store not set")
	}
	if !p.storeStale {
		t.Fatal("expected store_stale=true")
	}
}

func TestMiddleware_MissingCache_PlatformOutage_404(t *testing.T) {
	repo := newFakeRepo()
	client := &fakeClient{err: stores.ErrPlatformUnavailable}
	p := &probe{}
	r := buildRouter(baseCfg(repo, client), p)

	w := doRequest(r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want 404", w.Code)
	}
	if p.reached {
		t.Fatal("handler should not be reached")
	}
}

func TestMiddleware_StaleCache_BeyondCeiling_404(t *testing.T) {
	repo := newFakeRepo()
	repo.preload(newFixtureStore(time.Now().Add(-25 * time.Hour)))
	client := &fakeClient{err: stores.ErrPlatformUnavailable}
	p := &probe{}
	r := buildRouter(baseCfg(repo, client), p)

	w := doRequest(r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want 404", w.Code)
	}
	if p.reached {
		t.Fatal("handler should not be reached")
	}
}

// TestMiddleware_MarkStaleForTenant_PlatformOutage_ServesStale pins the
// fail-open requirement (F2, #287): MarkStaleForTenant backdates synced_at
// enough to trip IsStale and force a refresh attempt, but NOT so far back
// that a failed refresh (platform-api unreachable) falls outside StaleCeil.
// If MarkStaleForTenant instead wrote the epoch, this request would 404 —
// locking every merchant out of every store during an outage that follows
// an unsuspend, which is exactly the failure mode this design forbids.
func TestMiddleware_MarkStaleForTenant_PlatformOutage_ServesStale(t *testing.T) {
	repo := newFakeRepo()
	repo.preload(newFixtureStore(time.Now())) // fresh to start
	if err := repo.MarkStaleForTenant(context.Background(), testTenantID); err != nil {
		t.Fatalf("MarkStaleForTenant: %v", err)
	}

	client := &fakeClient{err: stores.ErrPlatformUnavailable}
	p := &probe{}
	r := buildRouter(baseCfg(repo, client), p)

	w := doRequest(r)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (fail-open on a stale-but-recent row)", w.Code)
	}
	if !p.reached {
		t.Fatal("handler not reached")
	}
	if !p.storeStale {
		t.Fatal("expected store_stale=true")
	}
}

func TestMiddleware_SingleflightCoalescing(t *testing.T) {
	repo := newFakeRepo()
	client := &fakeClient{
		result: newFixtureStore(time.Time{}),
		delay:  50 * time.Millisecond,
	}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", testTenantID) })
	r.GET("/stores/:storeId", stores.StoreMiddleware(baseCfg(repo, client)), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			w := doRequest(r)
			if w.Code != http.StatusOK {
				t.Errorf("status: got %d want 200", w.Code)
			}
		}()
	}
	wg.Wait()

	if got := client.calls.Load(); got != 1 {
		t.Fatalf("singleflight: client calls got %d want 1", got)
	}
}

// tenantScopedClient always returns a store owned by ownerTenant, whatever
// tenant asks for it, and blocks inside GetStore until release is closed so a
// second request has a window to join the singleflight.
type tenantScopedClient struct {
	ownerTenant string
	entered     chan struct{}
	release     chan struct{}
	calls       atomic.Int64
	once        sync.Once
}

func (c *tenantScopedClient) GetStore(_ context.Context, _, storeID string) (*stores.Store, error) {
	c.calls.Add(1)
	c.once.Do(func() {
		close(c.entered)
		<-c.release
	})
	s := newFixtureStore(time.Now())
	s.ID = storeID
	s.TenantID = c.ownerTenant
	return s, nil
}

func (c *tenantScopedClient) GetStoreBySlug(_ context.Context, _ string) (*stores.Store, error) {
	return nil, stores.ErrPlatformUnavailable
}

func buildRouterForTenant(cfg stores.MiddlewareConfig, p *probe, tenantID string, onEntry func()) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("tenant_id", tenantID)
		if onEntry != nil {
			onEntry()
		}
	})
	r.GET("/stores/:storeId", stores.StoreMiddleware(cfg), func(c *gin.Context) {
		p.reached = true
		if v, ok := c.Get("store"); ok {
			p.store, _ = v.(*stores.Store)
		}
		c.String(http.StatusOK, "ok")
	})
	return r
}

// A concurrent request from another tenant must not piggyback on the
// singleflight refresh of the owning tenant and receive its store.
func TestStoreMiddleware_ConcurrentRefreshIsTenantScoped(t *testing.T) {
	const otherTenantID = "33333333-3333-3333-3333-333333333333"

	client := &tenantScopedClient{
		ownerTenant: testTenantID,
		entered:     make(chan struct{}),
		release:     make(chan struct{}),
	}
	flight := &singleflight.Group{}

	ownerProbe := &probe{}
	ownerCfg := baseCfg(newFakeRepo(), client)
	ownerCfg.Flight = flight
	ownerRouter := buildRouterForTenant(ownerCfg, ownerProbe, testTenantID, nil)

	otherProbe := &probe{}
	otherCfg := baseCfg(newFakeRepo(), client)
	otherCfg.Flight = flight
	otherEntered := make(chan struct{})
	otherRouter := buildRouterForTenant(otherCfg, otherProbe, otherTenantID, func() {
		close(otherEntered)
	})

	ownerDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { ownerDone <- doRequest(ownerRouter) }()

	<-client.entered

	otherDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { otherDone <- doRequest(otherRouter) }()

	<-otherEntered
	// Give the second request time to reach Flight.Do before the first
	// refresh completes; without it the singleflight window never opens.
	time.Sleep(50 * time.Millisecond)
	close(client.release)

	ownerRes := <-ownerDone
	otherRes := <-otherDone

	if ownerRes.Code != http.StatusOK {
		t.Fatalf("owning tenant: got %d, want 200", ownerRes.Code)
	}
	if otherRes.Code != http.StatusNotFound {
		t.Fatalf("foreign tenant: got %d, want 404", otherRes.Code)
	}
	if otherProbe.reached {
		t.Fatal("foreign tenant reached the handler")
	}
	if got := client.calls.Load(); got != 2 {
		t.Fatalf("platform client calls = %d, want 2 (one per tenant)", got)
	}
}
