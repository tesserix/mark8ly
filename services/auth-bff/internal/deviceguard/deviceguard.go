// Package deviceguard recognises whether a login came from a device
// the account has used before, and raises a security alert when it
// has not.
//
// The check is deliberately fail-open for the user and fail-loud for
// security: if device history cannot be read, the login still
// proceeds but is treated as unrecognised and alerted. Locking people
// out of their own accounts on a database blip is a worse outcome
// than an occasional redundant email.
package deviceguard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"time"

	"github.com/mark8ly/auth-bff/internal/geoip"
)

// Store answers whether an account has previously logged in from a
// given device fingerprint.
type Store interface {
	HasSeen(ctx context.Context, userID, fingerprint string) (bool, error)
}

// Alert is the payload handed to the notification channel.
type Alert struct {
	UserID      string
	Email       string
	Device      string
	IPAddress   string
	Country     string
	CountryName string
	At          time.Time
}

// Notifier delivers a new-device alert. Implementations must not block
// for long — Evaluate calls this on the login path.
type Notifier interface {
	NotifyNewDevice(ctx context.Context, a Alert) error
}

// Login is the set of facts about a single sign-in.
type Login struct {
	UserID      string
	Email       string
	Fingerprint string
	Device      string
	IPAddress   string
	Country     string
	At          time.Time
}

// Config holds Service dependencies. Notifier is optional.
type Config struct {
	Store    Store
	Notifier Notifier
	Logger   *slog.Logger
}

// Service evaluates logins against device history.
type Service struct {
	store    Store
	notifier Notifier
	logger   *slog.Logger
}

// NewService constructs a Service. Store is required.
func NewService(cfg Config) (*Service, error) {
	if cfg.Store == nil {
		return nil, errors.New("deviceguard: store is required")
	}
	return &Service{store: cfg.Store, notifier: cfg.Notifier, logger: cfg.Logger}, nil
}

// Fingerprint derives a stable, non-reversible device identifier from
// the client's user agent. It is a coarse signal, not an anti-fraud
// device ID — two identical browsers on identical OS versions collide,
// which errs toward fewer alerts rather than false alarms.
func Fingerprint(userAgent string) string {
	sum := sha256.Sum256([]byte(userAgent))
	return hex.EncodeToString(sum[:])
}

// Evaluate reports whether this login is from an unrecognised device
// and, when it is, dispatches an alert. Notification failures are
// logged, never returned — the caller is mid-login.
func (s *Service) Evaluate(ctx context.Context, l Login) (bool, error) {
	known := false

	// An absent fingerprint must never match history, or a client that
	// omits its user agent would inherit another device's trust.
	if l.Fingerprint != "" {
		seen, err := s.store.HasSeen(ctx, l.UserID, l.Fingerprint)
		if err != nil {
			s.log("deviceguard: device history unavailable — treating device as new",
				"err", err, "user_id", l.UserID)
		} else {
			known = seen
		}
	}
	if known {
		return false, nil
	}

	if s.notifier != nil {
		at := l.At
		if at.IsZero() {
			at = time.Now().UTC()
		}
		alert := Alert{
			UserID:      l.UserID,
			Email:       l.Email,
			Device:      l.Device,
			IPAddress:   l.IPAddress,
			Country:     l.Country,
			CountryName: geoip.Describe(l.Country),
			At:          at,
		}
		if err := s.notifier.NotifyNewDevice(ctx, alert); err != nil {
			s.log("deviceguard: new-device alert failed to send",
				"err", err, "user_id", l.UserID)
		}
	}
	return true, nil
}

func (s *Service) log(msg string, args ...any) {
	if s.logger != nil {
		s.logger.Warn(msg, args...)
	}
}
