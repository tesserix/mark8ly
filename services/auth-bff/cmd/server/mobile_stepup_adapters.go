package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/mark8ly/auth-bff/internal/emailotp"
	"github.com/mark8ly/auth-bff/internal/loginotp"
	"github.com/mark8ly/auth-bff/internal/session"
	"github.com/mark8ly/auth-bff/internal/zitadellogin"
)

// codeVerifierAdapter lets the browser challenge's Gate satisfy
// zitadellogin.CodeVerifier.
//
// An adapter rather than renaming the interface method to Verify: a
// method named `Verify(ctx, string, string) error` is generic enough that
// unrelated types would satisfy the interface by accident, and this is the
// one check standing between an emailed code and a bearer token.
type codeVerifierAdapter struct{ g *loginotp.Gate }

func (a codeVerifierAdapter) VerifyCode(ctx context.Context, email, code string) error {
	return a.g.Verify(ctx, email, code)
}

// challengeIssuerAdapter lets the browser challenge's Gate back the mobile
// resend (#686 item 3), translating the one issuer failure the handler has
// to answer differently.
//
// *loginotp.Gate already satisfies zitadellogin.ChallengeIssuer by
// signature; the adapter exists for the ERROR vocabulary. zitadellogin
// deliberately knows nothing about the OTP subsystem (see its CodeVerifier
// doc), so it cannot errors.Is against emailotp.ErrRateLimited itself.
// Without this translation a merchant who has spent their code budget
// would get a generic failure and keep tapping Resend against a wall.
type challengeIssuerAdapter struct{ g *loginotp.Gate }

func (a challengeIssuerAdapter) IssueChallenge(ctx context.Context, email, ip string) error {
	err := a.g.IssueChallenge(ctx, email, ip)
	if errors.Is(err, emailotp.ErrRateLimited) {
		return fmt.Errorf("%w: %w", zitadellogin.ErrChallengeRateLimited, err)
	}
	return err
}

// pendingStoreAdapter exposes the session manager's sealed-pending format
// to zitadellogin without that package importing session.
//
// The mobile step-up token and the browser's pending cookie are the same
// payload under the same key — only the delivery differs — so there is one
// format to reason about rather than two.
type pendingStoreAdapter struct{ m *session.Manager }

func (a pendingStoreAdapter) SealPendingLogin(p zitadellogin.PendingLogin) (string, error) {
	return a.m.SealPending(session.Pending{
		UID:                 p.UID,
		Email:               p.Email,
		TenantID:            p.TenantID,
		ZitadelSessionID:    p.ZitadelSessionID,
		ZitadelSessionToken: p.ZitadelSessionToken,
	})
}

func (a pendingStoreAdapter) OpenPendingLogin(value string) (*zitadellogin.PendingLogin, error) {
	sp, err := a.m.OpenPending(value)
	if err != nil {
		return nil, err
	}
	if sp == nil {
		return nil, nil
	}
	return &zitadellogin.PendingLogin{
		UID:                 sp.UID,
		Email:               sp.Email,
		TenantID:            sp.TenantID,
		ZitadelSessionID:    sp.ZitadelSessionID,
		ZitadelSessionToken: sp.ZitadelSessionToken,
	}, nil
}
