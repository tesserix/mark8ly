package verification

import (
	"context"
	"errors"
	"testing"
	"time"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// issueAndConsume returns a token that has been through VerifyToken once,
// i.e. exactly the state a replayed magic link is in.
func issueAndConsume(t *testing.T, repo *fakeRepo) string {
	t.Helper()
	// The raw token is only recoverable from the emailed link, so mint one
	// directly and store it in the state SendMagicLink + VerifyToken leave
	// behind: present, unexpired, already consumed.
	raw, hash, err := generateToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	now := time.Now()
	repo.tokens[hash] = &Token{
		ID:         "tok-replay",
		SessionID:  "sess-1",
		Email:      "user@example.com",
		CodeHash:   hash,
		ExpiresAt:  now.Add(TokenLifetime),
		ConsumedAt: &now,
	}
	return raw
}

func TestResolveReplay_ReturnsSessionForAConsumedTokenStillInDate(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, &captureSender{})
	raw := issueAndConsume(t, repo)

	res, err := svc.ResolveReplay(context.Background(), raw)
	if err != nil {
		t.Fatalf("ResolveReplay: %v", err)
	}
	if res.SessionID != "sess-1" || res.Email != "user@example.com" {
		t.Errorf("got %+v, want sess-1/user@example.com", res)
	}
}

// The boundary the issue did not ask for, and the reason this method
// enforces expiry: without it a spent 10-minute credential would answer
// with a session id and email forever, to anyone who later found the URL
// in a log, a browser history, or a scanner cache.
func TestResolveReplay_RefusesAConsumedTokenPastItsExpiry(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, &captureSender{})
	raw, hash, err := generateToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	consumed := time.Now().Add(-2 * TokenLifetime)
	repo.tokens[hash] = &Token{
		ID:         "tok-old",
		SessionID:  "sess-1",
		Email:      "user@example.com",
		CodeHash:   hash,
		ExpiresAt:  time.Now().Add(-TokenLifetime), // already lapsed
		ConsumedAt: &consumed,
	}

	_, err = svc.ResolveReplay(context.Background(), raw)
	var ae *apperrors.AppError
	if !errors.As(err, &ae) || ae.Code != "token_expired" {
		t.Fatalf("err = %v, want token_expired", err)
	}
}

func TestResolveReplay_RefusesATokenThatWasNeverConsumed(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, &captureSender{})
	raw, hash, err := generateToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	repo.tokens[hash] = &Token{
		ID: "tok-fresh", SessionID: "sess-1", Email: "user@example.com",
		CodeHash: hash, ExpiresAt: time.Now().Add(TokenLifetime),
	}

	if _, err := svc.ResolveReplay(context.Background(), raw); err == nil {
		t.Fatal("want error for an unconsumed token, got nil")
	}
}

func TestResolveReplay_NeverMutates(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, &captureSender{})
	raw := issueAndConsume(t, repo)

	before := *repo.tokens[hashToken(raw)]
	if _, err := svc.ResolveReplay(context.Background(), raw); err != nil {
		t.Fatalf("ResolveReplay: %v", err)
	}
	after := *repo.tokens[hashToken(raw)]

	if !before.ConsumedAt.Equal(*after.ConsumedAt) || !before.ExpiresAt.Equal(after.ExpiresAt) {
		t.Error("ResolveReplay mutated the token; it must be read-only")
	}
}

func TestVerifyTokenStillRefusesAReplay(t *testing.T) {
	// ResolveReplay is additive. VerifyToken itself must keep failing on a
	// consumed token, so nothing can be verified twice.
	repo := newFakeRepo()
	svc := newService(repo, &captureSender{})
	raw := issueAndConsume(t, repo)

	_, err := svc.VerifyToken(context.Background(), raw)
	var ae *apperrors.AppError
	if !errors.As(err, &ae) || ae.Code != ErrCodeTokenConsumed {
		t.Fatalf("err = %v, want %s", err, ErrCodeTokenConsumed)
	}
}
