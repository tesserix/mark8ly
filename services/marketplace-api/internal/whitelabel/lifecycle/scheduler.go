package lifecycle

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/robfig/cron/v3"
)

// AdvancerAPI is what Scheduler needs from Advancer. Interface extracted
// for test-time injectability (fakeAdvancer) without pulling a DB.
type AdvancerAPI interface {
	AdvanceDue(ctx context.Context) error
}

// Scheduler drives Advancer.AdvanceDue on a cron cadence. Production
// cadence is 05:00 UTC daily ("0 5 * * *"); tests pass "@every 1s" or
// similar for deterministic exercise.
//
// A single Scheduler instance is constructed at boot in main.go; Run
// blocks until ctx cancels or Stop is called externally.
type Scheduler struct {
	adv      AdvancerAPI
	cronSpec string
	logger   *slog.Logger
	sch      *cron.Cron
}

// NewScheduler wires the scheduler. Caller is responsible for calling
// Run (in a goroutine) to start the cron loop.
func NewScheduler(adv AdvancerAPI, cronSpec string, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{adv: adv, cronSpec: cronSpec, logger: logger}
}

// Run starts the cron loop and blocks until ctx is cancelled. Errors
// from AdvanceDue are logged, not propagated — the loop continues.
func (s *Scheduler) Run(ctx context.Context) error {
	s.sch = cron.New(cron.WithLocation(mustUTC()))
	_, err := s.sch.AddFunc(s.cronSpec, func() {
		if err := s.adv.AdvanceDue(ctx); err != nil {
			s.logger.ErrorContext(ctx, "lifecycle: advance tick failed", "err", err)
		}
	})
	if err != nil {
		return fmt.Errorf("lifecycle: invalid cron spec %q: %w", s.cronSpec, err)
	}
	s.sch.Start()
	<-ctx.Done()
	s.Stop()
	return nil
}

// Stop halts the underlying cron scheduler. Safe to call even if Run
// never started.
func (s *Scheduler) Stop() {
	if s.sch != nil {
		s.sch.Stop()
	}
}

// mustUTC returns the UTC location; never fails. Kept as a tiny helper
// for symmetry with the cron API expecting a *time.Location.
func mustUTC() *TimeLocation {
	return utcLoc
}
