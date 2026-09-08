package breakglass

import "context"

// DBLoginWindow is the estate-wide sliding window, backed by
// break_glass_login_attempts (#846).
//
// It is a thin adapter over the repository rather than its own query layer:
// the SQL lives beside every other break-glass query, and this type exists
// only to satisfy LoginWindow without making the handler depend on the whole
// Repository surface for three methods.
type DBLoginWindow struct{ repo *Repository }

// NewDBLoginWindow returns a LoginWindow backed by Postgres.
func NewDBLoginWindow(repo *Repository) *DBLoginWindow { return &DBLoginWindow{repo: repo} }

// RecordFailure appends the attempt and returns the in-window count.
func (w *DBLoginWindow) RecordFailure(ctx context.Context, k LoginKey) (int, error) {
	return w.repo.RecordLoginFailure(ctx, k.IPHash)
}

// Reset clears every attempt for this ip_hash.
func (w *DBLoginWindow) Reset(ctx context.Context, k LoginKey) error {
	return w.repo.ClearLoginFailures(ctx, k.IPHash)
}

// Count returns the in-window failure count.
func (w *DBLoginWindow) Count(ctx context.Context, k LoginKey) (int, error) {
	return w.repo.CountLoginFailures(ctx, k.IPHash)
}

// Compile-time proof both implementations satisfy the interface. Without
// these, a signature drifting on one of them would only fail at the wiring
// site in main.go, which is a long way from the code that broke.
var (
	_ LoginWindow = (*DBLoginWindow)(nil)
	_ LoginWindow = (*LoginRateLimiter)(nil)
)
