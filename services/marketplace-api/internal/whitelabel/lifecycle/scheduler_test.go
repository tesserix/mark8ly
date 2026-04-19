package lifecycle_test

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mark8ly/marketplace-api/internal/whitelabel/lifecycle"
)

// fakeAdvancer counts AdvanceDue invocations deterministically.
type fakeAdvancer struct{ count atomic.Int64 }

func (f *fakeAdvancer) AdvanceDue(context.Context) error {
	f.count.Add(1)
	return nil
}

func TestScheduler_InvokesAdvancerOnTick(t *testing.T) {
	adv := &fakeAdvancer{}
	// @every 1s triggers within 1-2s.
	sched := lifecycle.NewScheduler(adv, "@every 1s", slog.Default())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- sched.Run(ctx) }()

	// Wait for at least one tick.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if adv.count.Load() >= 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	cancel()

	select {
	case <-errCh:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancel")
	}

	if adv.count.Load() < 1 {
		t.Errorf("advancer tick count = %d, want >= 1", adv.count.Load())
	}
}

func TestScheduler_InvalidCronSpec_ReturnsError(t *testing.T) {
	adv := &fakeAdvancer{}
	sched := lifecycle.NewScheduler(adv, "not a valid cron spec", slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sched.Run(ctx); err == nil {
		t.Error("Run with invalid spec = nil, want error")
	}
}

func TestScheduler_Stop_SafeOnNilSched(t *testing.T) {
	adv := &fakeAdvancer{}
	sched := lifecycle.NewScheduler(adv, "0 5 * * *", nil)
	// Never called Run, so sch is nil — Stop must not panic.
	sched.Stop()
}
