// Package loginotp completes a sign-in that autologin left pending
// because the device was unrecognised.
//
// The split matters for security: autologin proves who you are (GIP
// token) and this package proves you control the account's inbox. A code
// on its own is never enough — every endpoint here requires the pending
// cookie autologin wrote, so an attacker holding only a stolen code has
// nothing to spend it on.
package loginotp

import (
	"context"
	"fmt"
	"time"

	"github.com/mark8ly/auth-bff/internal/emailotp"
)

// Mailer delivers the code. Implemented by internal/notify.
type Mailer interface {
	SendLoginCode(ctx context.Context, email, code string, ttl time.Duration) error
}

// Gate issues and verifies the one-time codes.
type Gate struct {
	otp    *emailotp.Service
	mailer Mailer
	ttl    time.Duration
}

// NewGate constructs a Gate. ttl is used for the email copy only; the
// authoritative expiry lives in the emailotp service.
func NewGate(otp *emailotp.Service, mailer Mailer, ttl time.Duration) *Gate {
	if ttl == 0 {
		ttl = emailotp.DefaultTTL
	}
	return &Gate{otp: otp, mailer: mailer, ttl: ttl}
}

// IssueChallenge mints a code and mails it. Implements
// autologin.ChallengeIssuer.
func (g *Gate) IssueChallenge(ctx context.Context, email, ip string) error {
	ch, err := g.otp.Request(ctx, email, ip)
	if err != nil {
		return fmt.Errorf("loginotp: request code: %w", err)
	}
	if err := g.mailer.SendLoginCode(ctx, email, ch.Code, g.ttl); err != nil {
		return fmt.Errorf("loginotp: send code: %w", err)
	}
	return nil
}

// Verify checks a submitted code against the address.
func (g *Gate) Verify(ctx context.Context, email, code string) error {
	return g.otp.Verify(ctx, email, code)
}
