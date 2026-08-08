// Package usersessions is the auth-bff registry of active user
// sessions. One row per successful autologin mint. Powers the admin
// "Active sessions" UI (GET /api/v1/sessions, DELETE /api/v1/sessions/:id).
//
// Storage is a Postgres table (see migrations/0002_user_sessions.up.sql).
// The session JWT cookie itself stays the source of truth for auth —
// this table is a side-registry for the UI and for targeted revocation.
package usersessions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Session is a single row in user_sessions.
type Session struct {
	ID           string
	UserID       string
	TenantID     string
	Device       string
	IPAddress    string
	UserAgent    string
	LastActiveAt time.Time
	CreatedAt    time.Time
	RevokedAt    *time.Time
}

// Repository encapsulates DB access for the user_sessions table.
type Repository struct {
	db *sql.DB
}

// NewRepository constructs a Repository. db must be non-nil.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// CreateParams are the fields captured at login time.
type CreateParams struct {
	UserID    string
	TenantID  string
	Device    string
	IPAddress string
	UserAgent string
	// Fingerprint is the deviceguard hash of the user agent. Recording
	// it here is what makes the next login from this device recognised.
	Fingerprint string
}

// Create inserts a new session row and returns the generated ID.
func (r *Repository) Create(ctx context.Context, p CreateParams) (string, error) {
	if r == nil || r.db == nil {
		return "", nil // no-op if registry is not wired
	}
	id := uuid.NewString()
	const q = `
		INSERT INTO user_sessions (id, user_id, tenant_id, device, ip_address, user_agent, fingerprint)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	if _, err := r.db.ExecContext(ctx, q, id, p.UserID, p.TenantID, p.Device, p.IPAddress, p.UserAgent, p.Fingerprint); err != nil {
		return "", fmt.Errorf("usersessions: create: %w", err)
	}
	return id, nil
}

// Touch updates last_active_at for a session. Silently succeeds if the
// row doesn't exist — session lookup via ID happens at the JWT layer,
// so a missing row here just means the session was minted before the
// registry existed.
func (r *Repository) Touch(ctx context.Context, id string) error {
	if r == nil || r.db == nil {
		return nil
	}
	const q = `UPDATE user_sessions SET last_active_at = NOW() WHERE id = $1 AND revoked_at IS NULL`
	_, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("usersessions: touch: %w", err)
	}
	return nil
}

// ListForUser returns every non-revoked session for the user, newest first.
func (r *Repository) ListForUser(ctx context.Context, userID string) ([]Session, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	const q = `
		SELECT id, user_id, tenant_id, device, ip_address, user_agent, last_active_at, created_at, revoked_at
		FROM user_sessions
		WHERE user_id = $1 AND revoked_at IS NULL
		ORDER BY last_active_at DESC
	`
	rows, err := r.db.QueryContext(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("usersessions: list: %w", err)
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.UserID, &s.TenantID, &s.Device, &s.IPAddress, &s.UserAgent, &s.LastActiveAt, &s.CreatedAt, &s.RevokedAt); err != nil {
			return nil, fmt.Errorf("usersessions: scan: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("usersessions: rows: %w", err)
	}
	return out, nil
}

// ErrNotOwned is returned when a Revoke target belongs to a different user.
var ErrNotOwned = errors.New("usersessions: session does not belong to user")

// Revoke marks a single session revoked. Must match user_id — a user can
// only revoke their own sessions. Returns ErrNotOwned if the row exists
// but belongs to someone else, sql.ErrNoRows if it doesn't exist.
func (r *Repository) Revoke(ctx context.Context, id, userID string) error {
	if r == nil || r.db == nil {
		return nil
	}
	const q = `
		UPDATE user_sessions
		SET revoked_at = NOW()
		WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL
	`
	res, err := r.db.ExecContext(ctx, q, id, userID)
	if err != nil {
		return fmt.Errorf("usersessions: revoke: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("usersessions: rows affected: %w", err)
	}
	if n == 0 {
		// Distinguish "doesn't belong to user" from "already revoked / missing"
		var existsForOtherUser bool
		_ = r.db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM user_sessions WHERE id = $1 AND user_id <> $2)`,
			id, userID,
		).Scan(&existsForOtherUser)
		if existsForOtherUser {
			return ErrNotOwned
		}
		return sql.ErrNoRows
	}
	return nil
}

// RevokeAllForUser marks every active session for the user as revoked.
// Used by logout and by the delete-account flow.
func (r *Repository) RevokeAllForUser(ctx context.Context, userID string) error {
	if r == nil || r.db == nil {
		return nil
	}
	const q = `UPDATE user_sessions SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL`
	if _, err := r.db.ExecContext(ctx, q, userID); err != nil {
		return fmt.Errorf("usersessions: revoke all: %w", err)
	}
	return nil
}
