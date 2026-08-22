package domain

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// spyGIPKey captures calls so the test can assert the domain.Service
// fired the right method with the right FQDN.
type spyGIPKey struct {
	mu      sync.Mutex
	added   []string
	removed []string
	addErr  error
	rmErr   error
}

func (s *spyGIPKey) AddDomain(_ context.Context, d string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.added = append(s.added, d)
	return s.addErr
}

func (s *spyGIPKey) RemoveDomain(_ context.Context, d string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removed = append(s.removed, d)
	return s.rmErr
}

// memRepo is the in-memory Repository used by these tests. It is the
// minimum surface that markVerified / Remove touch — Update and
// Delete. The rest panics so an accidental call is loud rather than
// silent.
type memRepo struct {
	mu   sync.Mutex
	rows map[uuid.UUID]*CustomDomain
}

func newMemRepo(d *CustomDomain) *memRepo {
	r := &memRepo{rows: map[uuid.UUID]*CustomDomain{}}
	if d != nil {
		r.rows[d.ID] = d
	}
	return r
}

func (r *memRepo) List(_ context.Context, _ *gorm.DB, _ uuid.UUID) ([]CustomDomain, error) {
	panic("not implemented")
}

func (r *memRepo) GetByID(_ context.Context, _ *gorm.DB, _ uuid.UUID, id uuid.UUID) (*CustomDomain, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.rows[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *d
	return &cp, nil
}

func (r *memRepo) Create(_ context.Context, _ *gorm.DB, _ *CustomDomain) error {
	panic("not implemented")
}

func (r *memRepo) Update(_ context.Context, _ *gorm.DB, d *CustomDomain) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *d
	r.rows[d.ID] = &cp
	return nil
}

func (r *memRepo) Delete(_ context.Context, _ *gorm.DB, _ uuid.UUID, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rows, id)
	return nil
}

func (r *memRepo) ListByStatus(_ context.Context, _ *gorm.DB, _ DomainStatus) ([]CustomDomain, error) {
	panic("not implemented")
}

func (r *memRepo) GetByDomain(_ context.Context, _ *gorm.DB, _ string) (*CustomDomain, error) {
	panic("not implemented")
}

func newSilentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestService(t *testing.T, repo Repository, spy *spyGIPKey) *Service {
	t.Helper()
	return NewService(ServiceConfig{
		// DB is unused when the repo doesn't read from it.
		DB:     nil,
		Repo:   repo,
		GIPKey: spy,
		Logger: newSilentLogger(),
	})
}

func TestMarkVerified_FiresGIPKeyAddDomain(t *testing.T) {
	t.Parallel()

	d := &CustomDomain{
		ID:        uuid.New(),
		TenantID:  uuid.New(),
		StoreID:   uuid.New(),
		Domain:    "primasyss.com",
		DNSMethod: DNSMethodManual,
		Status:    DomainStatusVerifying,
		SSLStatus: SSLStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	repo := newMemRepo(d)
	spy := &spyGIPKey{}
	svc := newTestService(t, repo, spy)

	if _, err := svc.markVerified(context.Background(), d); err != nil {
		t.Fatalf("markVerified: %v", err)
	}
	spy.mu.Lock()
	defer spy.mu.Unlock()
	if len(spy.added) != 1 || spy.added[0] != "primasyss.com" {
		t.Fatalf("AddDomain calls = %v, want [primasyss.com]", spy.added)
	}
}

func TestMarkVerified_GIPKeyFailureDoesNotFailVerify(t *testing.T) {
	t.Parallel()

	d := &CustomDomain{
		ID:        uuid.New(),
		TenantID:  uuid.New(),
		StoreID:   uuid.New(),
		Domain:    "primasyss.com",
		DNSMethod: DNSMethodManual,
		Status:    DomainStatusVerifying,
		SSLStatus: SSLStatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	repo := newMemRepo(d)
	spy := &spyGIPKey{addErr: errors.New("quota exceeded")}
	svc := newTestService(t, repo, spy)

	out, err := svc.markVerified(context.Background(), d)
	if err != nil {
		t.Fatalf("markVerified must not fail on gipkey error, got: %v", err)
	}
	if out.Status != DomainStatusActive {
		t.Fatalf("status = %s, want active", out.Status)
	}
}

func TestRemove_FiresGIPKeyRemoveDomain(t *testing.T) {
	t.Parallel()

	d := &CustomDomain{
		ID:        uuid.New(),
		TenantID:  uuid.New(),
		StoreID:   uuid.New(),
		Domain:    "primasyss.com",
		DNSMethod: DNSMethodManual,
		Status:    DomainStatusActive,
		SSLStatus: SSLStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	repo := newMemRepo(d)
	spy := &spyGIPKey{}
	svc := newTestService(t, repo, spy)

	if err := svc.Remove(context.Background(), d.StoreID, d.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	spy.mu.Lock()
	defer spy.mu.Unlock()
	if len(spy.removed) != 1 || spy.removed[0] != "primasyss.com" {
		t.Fatalf("RemoveDomain calls = %v, want [primasyss.com]", spy.removed)
	}
}
