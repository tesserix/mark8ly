package deviceguard

import (
	"context"
	"database/sql"
	"fmt"
)

// SessionStore answers device-history questions from the existing
// user_sessions registry. Revoked sessions still count as history: a
// device the user signed out of is one they have used before, so
// returning to it is not a security event.
type SessionStore struct {
	db *sql.DB
}

// NewSessionStore wraps a sql.DB handle.
func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}

func (s *SessionStore) HasSeen(ctx context.Context, userID, fingerprint string) (bool, error) {
	if s == nil || s.db == nil {
		return false, sql.ErrConnDone
	}
	const q = `
		SELECT EXISTS(
			SELECT 1 FROM user_sessions
			WHERE user_id = $1 AND fingerprint = $2
		)
	`
	var seen bool
	if err := s.db.QueryRowContext(ctx, q, userID, fingerprint).Scan(&seen); err != nil {
		return false, fmt.Errorf("device history lookup: %w", err)
	}
	return seen, nil
}
