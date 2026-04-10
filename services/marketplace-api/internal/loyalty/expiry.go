package loyalty

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ExpiryWorker runs a daily cron that expires loyalty points whose
// earn transaction is older than the program's point_expiry_days.
// Uses the csvjob worker pattern: context-controlled goroutine with
// ticker, batch of 500, FOR UPDATE SKIP LOCKED.
type ExpiryWorker struct {
	db     *gorm.DB
	repo   Repository
	logger *slog.Logger
}

// NewExpiryWorker constructs an ExpiryWorker.
func NewExpiryWorker(db *gorm.DB, repo Repository, logger *slog.Logger) *ExpiryWorker {
	return &ExpiryWorker{db: db, repo: repo, logger: logger}
}

// Start launches the expiry polling loop. Returns a channel that closes
// when the worker exits (mirrors the csvjob pattern). The caller passes
// a cancellable context to shut the worker down gracefully.
func (w *ExpiryWorker) Start(ctx context.Context, interval time.Duration) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.logger.Info("loyalty: expiry worker started", "interval", interval)

		// Run once immediately on startup
		w.runCycle(ctx)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				w.logger.Info("loyalty: expiry worker stopping")
				return
			case <-ticker.C:
				w.runCycle(ctx)
			}
		}
	}()
	return done
}

// runCycle processes all stores that have point_expiry_days configured,
// then expires points in batches.
func (w *ExpiryWorker) runCycle(ctx context.Context) {
	// Find all programs with expiry configured
	var programs []LoyaltyProgram
	err := w.db.WithContext(ctx).
		Where("is_active = ? AND point_expiry_days IS NOT NULL", true).
		Find(&programs).Error
	if err != nil {
		w.logger.Error("loyalty: expiry cycle failed to load programs", "err", err)
		return
	}

	for _, program := range programs {
		if program.PointExpiryDays == nil {
			continue
		}
		expiryBefore := time.Now().AddDate(0, 0, -*program.PointExpiryDays)
		w.expireForProgram(ctx, program, expiryBefore)
	}
}

func (w *ExpiryWorker) expireForProgram(ctx context.Context, program LoyaltyProgram, expiryBefore time.Time) {
	const batchSize = 500
	for {
		if ctx.Err() != nil {
			return
		}

		// Amendment FIX 3: each batch in its own transaction, select
		// individual rows with FOR UPDATE SKIP LOCKED, aggregate in Go.
		var rows []ExpiredTransaction
		err := w.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			rows, err = w.repo.SelectExpiredTransactions(ctx, tx, expiryBefore, batchSize)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				return nil
			}

			// Aggregate by loyalty_id in Go
			type batch struct {
				loyaltyID uuid.UUID
				tenantID  uuid.UUID
				total     int
				txnIDs    []uuid.UUID
			}
			byLoyalty := make(map[uuid.UUID]*batch)
			for _, row := range rows {
				b, ok := byLoyalty[row.LoyaltyID]
				if !ok {
					b = &batch{loyaltyID: row.LoyaltyID, tenantID: row.TenantID}
					byLoyalty[row.LoyaltyID] = b
				}
				b.total += row.Points
				b.txnIDs = append(b.txnIDs, row.ID)
			}

			for _, b := range byLoyalty {
				if b.total <= 0 {
					continue
				}
				newBalance, err := w.repo.DebitPoints(tx, b.loyaltyID, b.total)
				if err != nil {
					// If insufficient points (customer already redeemed some), skip
					w.logger.Warn("loyalty: expiry debit partial — customer may have redeemed",
						"loyalty_id", b.loyaltyID, "attempted", b.total)
					continue
				}
				desc := fmt.Sprintf("Points expired (%d points)", b.total)
				if err := w.repo.CreateTransaction(tx, &LoyaltyTransaction{
					TenantID:     b.tenantID,
					LoyaltyID:    b.loyaltyID,
					Type:         TxTypeExpire,
					Points:       -b.total,
					BalanceAfter: newBalance,
					Description:  &desc,
					CreatedAt:    time.Now(),
				}); err != nil {
					w.logger.Error("loyalty: expiry create txn failed",
						"loyalty_id", b.loyaltyID, "err", err)
				}
			}
			return nil
		})
		if err != nil {
			w.logger.Error("loyalty: expiry batch failed",
				"store_id", program.StoreID, "err", err)
			return
		}

		if len(rows) < batchSize {
			return // last page
		}
	}
}
