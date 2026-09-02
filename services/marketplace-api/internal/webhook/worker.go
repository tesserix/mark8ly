package webhook

import (
	"context"
	"log/slog"
	"time"
)

const (
	// MaxAttempts before a delivery is dead-lettered.
	MaxAttempts = 6
	// FailureThreshold of CONSECUTIVE failures before the subscription is
	// disabled and the merchant told. Higher than MaxAttempts so one bad
	// delivery cannot disable a working endpoint — it takes sustained
	// failure across different events.
	FailureThreshold = 10
	// RetentionWindow for delivery rows. 30 days on every plan.
	RetentionWindow = 30 * 24 * time.Hour
)

// Worker drains webhook_deliveries.
type Worker struct {
	deliveries *DeliveryRepo
	subs       *SubscriptionRepo
	sender     *Sender
	logger     *slog.Logger
	batch      int
	notify     func(sub Subscription)
}

func NewWorker(deliveries *DeliveryRepo, subs *SubscriptionRepo, sender *Sender, logger *slog.Logger, batch int, notify func(Subscription)) *Worker {
	if batch <= 0 {
		batch = 4 // bounded: these goroutines share a pod with API traffic
	}
	return &Worker{deliveries: deliveries, subs: subs, sender: sender, logger: logger, batch: batch, notify: notify}
}

// Tick sends one batch. Returns how many deliveries were attempted.
func (w *Worker) Tick(ctx context.Context) (int, error) {
	due, err := w.deliveries.ClaimDue(ctx, w.batch)
	if err != nil {
		return 0, err
	}
	for _, d := range due {
		w.attempt(ctx, d)
	}
	return len(due), nil
}

func (w *Worker) attempt(ctx context.Context, d Delivery) {
	sub, err := w.subs.ByID(ctx, d.SubscriptionID)
	if err != nil || sub == nil {
		return // subscription deleted mid-flight; the cascade will clean up
	}

	code, sendErr := w.sender.Send(ctx, *sub, d)
	var codePtr *int
	if code != 0 {
		codePtr = &code
	}

	if sendErr == nil {
		if err := w.deliveries.RecordOutcome(ctx, d.ID, StatusDelivered, codePtr, nil, time.Now()); err != nil && w.logger != nil {
			w.logger.Error("webhook: record delivered failed", "err", err)
		}
		if err := w.subs.RecordSuccess(ctx, sub.ID); err != nil && w.logger != nil {
			w.logger.Error("webhook: record success failed", "err", err)
		}
		return
	}

	// Never log the endpoint's response body — arbitrary remote content.
	msg := truncate(sendErr.Error(), maxErrorLen)
	attempts := d.Attempts + 1
	status, next := StatusPending, time.Now().Add(backoff(attempts))
	if attempts >= MaxAttempts {
		status, next = StatusFailed, time.Now()
	}
	if err := w.deliveries.RecordOutcome(ctx, d.ID, status, codePtr, &msg, next); err != nil && w.logger != nil {
		w.logger.Error("webhook: record failure failed", "err", err)
	}

	if status != StatusFailed {
		return
	}
	w.disableIfExhausted(ctx, *sub)
}

func (w *Worker) disableIfExhausted(ctx context.Context, sub Subscription) {
	disabled, err := w.subs.RecordFailure(ctx, sub.ID, FailureThreshold)
	if err != nil && w.logger != nil {
		w.logger.Error("webhook: record subscription failure", "err", err)
	}
	if !disabled {
		return
	}
	if w.logger != nil {
		w.logger.Warn("webhook subscription auto-disabled",
			slog.String("subscription_id", sub.ID.String()))
	}
	if w.notify != nil {
		w.notify(sub)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Start runs Tick on an interval until ctx is cancelled.
func (w *Worker) Start(ctx context.Context, interval time.Duration) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := w.Tick(ctx); err != nil && w.logger != nil {
					w.logger.Error("webhook worker tick failed", "err", err)
				}
			}
		}
	}()
	return done
}
