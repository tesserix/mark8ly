package emailotp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// PostgresStore is the user_email_otp-backed Store.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore wraps a sql.DB handle.
func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) Insert(ctx context.Context, r Record) error {
	const q = `
		INSERT INTO user_email_otp
			(id, email, code_hash, ip_address, attempts, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := s.db.ExecContext(ctx, q,
		r.ID, r.Email, r.CodeHash, r.IPAddress, r.Attempts, r.ExpiresAt, r.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert challenge: %w", err)
	}
	return nil
}

func (s *PostgresStore) Latest(ctx context.Context, email string) (*Record, error) {
	const q = `
		SELECT id, email, code_hash, ip_address, attempts, expires_at, consumed_at, created_at
		FROM user_email_otp
		WHERE email = $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	var r Record
	err := s.db.QueryRowContext(ctx, q, email).Scan(
		&r.ID, &r.Email, &r.CodeHash, &r.IPAddress,
		&r.Attempts, &r.ExpiresAt, &r.ConsumedAt, &r.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoChallenge
	}
	if err != nil {
		return nil, fmt.Errorf("load latest challenge: %w", err)
	}
	return &r, nil
}

func (s *PostgresStore) IncrementAttempts(ctx context.Context, id string) error {
	const q = `UPDATE user_email_otp SET attempts = attempts + 1 WHERE id = $1`
	if _, err := s.db.ExecContext(ctx, q, id); err != nil {
		return fmt.Errorf("increment attempts: %w", err)
	}
	return nil
}

// Consume marks the challenge spent. The consumed_at IS NULL predicate
// makes single-use atomic: two concurrent verifications of the same
// valid code race here and exactly one wins.
func (s *PostgresStore) Consume(ctx context.Context, id string, at time.Time) error {
	const q = `UPDATE user_email_otp SET consumed_at = $2 WHERE id = $1 AND consumed_at IS NULL`
	res, err := s.db.ExecContext(ctx, q, id, at)
	if err != nil {
		return fmt.Errorf("consume challenge: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("consume rows affected: %w", err)
	}
	if n == 0 {
		return ErrAlreadyUsed
	}
	return nil
}

func (s *PostgresStore) CountSince(ctx context.Context, email string, since time.Time) (int, error) {
	const q = `SELECT COUNT(*) FROM user_email_otp WHERE email = $1 AND created_at > $2`
	var n int
	if err := s.db.QueryRowContext(ctx, q, email, since).Scan(&n); err != nil {
		return 0, fmt.Errorf("count recent challenges: %w", err)
	}
	return n, nil
}

// PurgeExpired deletes challenges past their usefulness. Called on a
// timer from main so the table stays small under login load.
func (s *PostgresStore) PurgeExpired(ctx context.Context, olderThan time.Time) (int64, error) {
	const q = `DELETE FROM user_email_otp WHERE expires_at < $1`
	res, err := s.db.ExecContext(ctx, q, olderThan)
	if err != nil {
		return 0, fmt.Errorf("purge expired challenges: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
