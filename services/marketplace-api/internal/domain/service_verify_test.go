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

func newTestService(t *testing.T, repo Repository) *Service {
	t.Helper()
	return NewService(ServiceConfig{
		// DB is unused when the repo doesn't read from it.
		DB:     nil,
		Repo:   repo,
		Logger: newSilentLogger(),
	})
}

func newTestDomain(status DomainStatus, ssl SSLStatus) *CustomDomain {
	return &CustomDomain{
		ID:        uuid.New(),
		TenantID:  uuid.New(),
		StoreID:   uuid.New(),
		Domain:    "primasyss.com",
		DNSMethod: DNSMethodManual,
		Status:    status,
		SSLStatus: ssl,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func TestMarkVerified_ActivatesAndPersists(t *testing.T) {
	t.Parallel()

	d := newTestDomain(DomainStatusVerifying, SSLStatusPending)
	repo := newMemRepo(d)
	svc := newTestService(t, repo)

	out, err := svc.markVerified(context.Background(), d)
	if err != nil {
		t.Fatalf("markVerified: %v", err)
	}
	if out.Status != DomainStatusActive {
		t.Fatalf("status = %s, want active", out.Status)
	}
	if out.VerifiedAt == nil {
		t.Fatal("VerifiedAt not stamped")
	}
	if out.SSLStatus != SSLStatusPending {
		t.Fatalf("ssl status = %s, want pending", out.SSLStatus)
	}

	// The transition must be durable, not just mutated in memory.
	stored, err := repo.GetByID(context.Background(), nil, d.StoreID, d.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored.Status != DomainStatusActive || stored.VerifiedAt == nil {
		t.Fatalf("stored row not verified: %+v", stored)
	}
}

func TestRemove_DeletesRow(t *testing.T) {
	t.Parallel()

	d := newTestDomain(DomainStatusActive, SSLStatusActive)
	repo := newMemRepo(d)
	svc := newTestService(t, repo)

	if err := svc.Remove(context.Background(), d.StoreID, d.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := repo.GetByID(context.Background(), nil, d.StoreID, d.ID); err == nil {
		t.Fatal("row still present after Remove")
	}
}
