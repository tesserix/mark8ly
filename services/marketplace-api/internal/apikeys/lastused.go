package apikeys

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/ipprivacy"
)

// lastUsedUpdate is the channel payload from middleware → worker.
type lastUsedUpdate struct {
	keyID  uuid.UUID
	at     time.Time
	ipHash string
}

// LastUsedWorker drains a buffered channel of (key_id, time, ip_hash)
// updates and writes them to the DB asynchronously. Lossy on shutdown:
// pending updates in the channel buffer are dropped at Stop time. A lost
// last_used observation is acceptable; an audit-grade pipeline would belong
// in `internal/audit/`, not here.
type LastUsedWorker struct {
	repo   *Repo
	hasher *ipprivacy.Hasher
	logger *slog.Logger
	ch     chan lastUsedUpdate
	wg     sync.WaitGroup
	stop   chan struct{}
}

// NewLastUsedWorker constructs and starts the worker. bufferSize bounds the
// in-memory queue; default 1024 if 0 passed. hasher may be nil — IP fields
// will then write empty.
func NewLastUsedWorker(repo *Repo, hasher *ipprivacy.Hasher, logger *slog.Logger, bufferSize int) *LastUsedWorker {
	if bufferSize <= 0 {
		bufferSize = 1024
	}
	w := &LastUsedWorker{
		repo:   repo,
		hasher: hasher,
		logger: logger,
		ch:     make(chan lastUsedUpdate, bufferSize),
		stop:   make(chan struct{}),
	}
	w.wg.Add(1)
	go w.run()
	return w
}

// Submit enqueues an update. Non-blocking; drops on full buffer (logged).
// Caller passes the raw client IP — the worker hashes it before write.
func (w *LastUsedWorker) Submit(keyID uuid.UUID, ip string) {
	if w == nil {
		return
	}
	hash := ""
	if w.hasher != nil {
		hash = w.hasher.Hash(ip)
	}
	select {
	case w.ch <- lastUsedUpdate{keyID: keyID, at: time.Now().UTC(), ipHash: hash}:
	default:
		if w.logger != nil {
			w.logger.Warn("apikeys: last-used buffer full, dropping update", "key_id", keyID)
		}
	}
}

// Stop signals the worker to drain the channel and exit. Bounded by ctx; if
// the context expires the worker logs and returns.
func (w *LastUsedWorker) Stop(ctx context.Context) {
	if w == nil {
		return
	}
	close(w.stop)
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		if w.logger != nil {
			w.logger.Warn("apikeys: last-used worker shutdown timed out")
		}
	}
}

func (w *LastUsedWorker) run() {
	defer w.wg.Done()
	for {
		select {
		case upd := <-w.ch:
			w.write(upd)
		case <-w.stop:
			// Drain remaining buffered updates.
			for {
				select {
				case upd := <-w.ch:
					w.write(upd)
				default:
					return
				}
			}
		}
	}
}

func (w *LastUsedWorker) write(upd lastUsedUpdate) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.repo.UpdateLastUsed(ctx, upd.keyID, upd.at, upd.ipHash); err != nil && w.logger != nil {
		w.logger.Warn("apikeys: last-used write failed", "key_id", upd.keyID, "err", err)
	}
}
