package lifecycle

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// captureRepo records the audit rows the Emitter's worker writes,
// standing in for the database so the emit path can be asserted without
// one.
type captureRepo struct {
	mu      sync.Mutex
	entries []audit.Entry
}

func (r *captureRepo) Create(_ context.Context, _ *gorm.DB, e *audit.Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, *e)
	return nil
}

func (r *captureRepo) actions() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e.Action)
	}
	return out
}

func (r *captureRepo) List(context.Context, *gorm.DB, audit.ListFilter) (audit.ListResult, error) {
	return audit.ListResult{}, nil
}

func (r *captureRepo) Stream(context.Context, *gorm.DB, audit.ListFilter, func(*audit.Entry) error) error {
	return nil
}

func (r *captureRepo) ListPlatform(context.Context, *gorm.DB, audit.PlatformListFilter) (audit.ListResult, error) {
	return audit.ListResult{}, nil
}

// stubNotifier records the calls the finalize cron makes and can fail.
type stubNotifier struct {
	err    error
	calls  int
	tenant uuid.UUID
	store  uuid.UUID
}

func (s *stubNotifier) ProAppCancelled(_ context.Context, tenantID, storeID uuid.UUID) error {
	s.calls++
	s.tenant, s.store = tenantID, storeID
	return s.err
}

func newCapturingCron(t *testing.T, n ProAppTeardownNotifier) (*FinalizeCron, *captureRepo) {
	t.Helper()
	repo := &captureRepo{}
	em, err := audit.NewEmitter(audit.EmitterConfig{Repo: repo, Logger: slog.Default()})
	require.NoError(t, err)
	// No t.Cleanup Stop: every test drains explicitly, and Emitter.Stop
	// closes a channel, so a second call panics.
	cron := NewFinalizeCron(nil, em, slog.Default(), nil).WithProAppTeardownNotifier(n)
	return cron, repo
}

// drain stops the emitter so its async worker has written everything
// before assertions read the capture.
func drain(t *testing.T, em *audit.Emitter) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*audit.WriteTimeout)
	defer cancel()
	em.Stop(ctx)
}

func proAppRow() *subscription.StoreSubscription {
	return &subscription.StoreSubscription{
		TenantID:              uuid.New(),
		StoreID:               uuid.New(),
		HasWhiteLabelAppAddOn: true,
	}
}

// The audit event is the record that a Pro+App cancellation happened. A
// teardown that cannot be seeded must not also erase it.
func TestOnProAppCancelled_EmitsEvenWhenDiscoveryFails(t *testing.T) {
	n := &stubNotifier{err: errors.New("boom: App Store Connect saw two apps")}
	cron, repo := newCapturingCron(t, n)

	row := proAppRow()
	cron.onProAppCancelled(context.Background(), row)
	drain(t, cron.emitter)

	require.Equal(t, 1, n.calls, "the notifier must still be called")
	require.Contains(t, repo.actions(), ActionProAppCancelled,
		"the cancellation audit event must be emitted even though teardown seeding failed")
}

func TestOnProAppCancelled_EmitsAndNotifiesOnSuccess(t *testing.T) {
	n := &stubNotifier{}
	cron, repo := newCapturingCron(t, n)

	row := proAppRow()
	cron.onProAppCancelled(context.Background(), row)
	drain(t, cron.emitter)

	require.Equal(t, 1, n.calls)
	require.Equal(t, row.TenantID, n.tenant)
	require.Equal(t, row.StoreID, n.store)
	require.Contains(t, repo.actions(), ActionProAppCancelled)
}

// A nil notifier is a wiring state, not a crash: the event is still
// recorded and the missing path is warned about.
func TestOnProAppCancelled_NoNotifierWired(t *testing.T) {
	cron, repo := newCapturingCron(t, nil)

	require.NotPanics(t, func() {
		cron.onProAppCancelled(context.Background(), proAppRow())
	})
	drain(t, cron.emitter)
	require.Contains(t, repo.actions(), ActionProAppCancelled)
}

// The notifier gets a bounded context so one hung App Store Connect call
// cannot stall the rest of the cohort's finalisation.
func TestOnProAppCancelled_NotifierContextHasDeadline(t *testing.T) {
	var deadline time.Time
	var ok bool
	cron, _ := newCapturingCron(t, notifierFunc(func(ctx context.Context, _, _ uuid.UUID) error {
		deadline, ok = ctx.Deadline()
		return nil
	}))

	cron.onProAppCancelled(context.Background(), proAppRow())
	drain(t, cron.emitter)

	require.True(t, ok, "notifier must be called with a deadline")
	require.WithinDuration(t, time.Now().Add(proAppNotifyTimeout), deadline, 5*time.Second)
}

type notifierFunc func(ctx context.Context, tenantID, storeID uuid.UUID) error

func (f notifierFunc) ProAppCancelled(ctx context.Context, tenantID, storeID uuid.UUID) error {
	return f(ctx, tenantID, storeID)
}
