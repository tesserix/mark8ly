package verification

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mark8ly/platform-api/internal/notification"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// fakeRepo is an in-memory verification.Repository for unit tests.
// Behavior mirrors the gorm implementation closely enough to exercise
// the Service contract without spinning up Postgres.
type fakeRepo struct {
	mu     sync.Mutex
	tokens map[string]*Token // keyed by hash
	nextID int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{tokens: make(map[string]*Token)}
}

func (r *fakeRepo) Create(_ context.Context, t *Token) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	if t.ID == "" {
		t.ID = string(rune('a' + r.nextID))
	}
	r.tokens[t.CodeHash] = t
	return nil
}

func (r *fakeRepo) GetByHash(_ context.Context, hash string) (*Token, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tokens[hash]
	if !ok {
		return nil, apperrors.NotFound("token_not_found", "verification link is invalid or expired")
	}
	return t, nil
}

func (r *fakeRepo) MarkConsumed(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.tokens {
		if t.ID == id && t.ConsumedAt == nil {
			now := time.Now()
			t.ConsumedAt = &now
			return nil
		}
	}
	return apperrors.NotFound("token_not_found", "verification token is not in a consumable state")
}

func (r *fakeRepo) InvalidateForSession(_ context.Context, sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for _, t := range r.tokens {
		if t.SessionID == sessionID && t.ConsumedAt == nil {
			t.ConsumedAt = &now
		}
	}
	return nil
}

// captureSender records every email passed to Send so tests can assert
// on the rendered payload (e.g. that the magic link contains the token).
type captureSender struct {
	mu   sync.Mutex
	last *notification.Email
}

func (s *captureSender) Send(_ context.Context, msg notification.Email) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := msg
	s.last = &cp
	return nil
}

func newService(repo Repository, sender notification.Sender) *Service {
	return NewService(repo, Config{
		Sender:        sender,
		EmailFrom:     "noreply@mark8ly.com",
		SupportEmail:  "help@mark8ly.com",
		VerifyURLBase: "https://onboarding.test",
	})
}

// TestSendMagicLink_HappyPath: SendMagicLink stores a token, sends an
// email containing the verification URL, and persists exactly one
// outstanding token for the session.
func TestSendMagicLink_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	sender := &captureSender{}
	svc := newService(repo, sender)

	if err := svc.SendMagicLink(context.Background(), "sess-1", "user@example.com", "Acme"); err != nil {
		t.Fatalf("send: %v", err)
	}

	if len(repo.tokens) != 1 {
		t.Fatalf("len(tokens) = %d, want 1", len(repo.tokens))
	}
	if sender.last == nil {
		t.Fatal("expected an email to be sent")
	}
	if sender.last.To != "user@example.com" {
		t.Errorf("To = %q", sender.last.To)
	}
	// The magic-link URL must appear in the body.
	if got := sender.last.HTMLBody; !contains(got, "https://onboarding.test/onboarding/verify?token=") {
		t.Errorf("HTML body missing magic link: %q", got)
	}
}

// TestSendMagicLink_InvalidatesPrevious: a second SendMagicLink for the
// same session must consume the prior token so only the latest link
// works (resend semantics).
func TestSendMagicLink_InvalidatesPrevious(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(repo, &captureSender{})
	ctx := context.Background()

	if err := svc.SendMagicLink(ctx, "sess-1", "user@example.com", ""); err != nil {
		t.Fatalf("first send: %v", err)
	}
	if err := svc.SendMagicLink(ctx, "sess-1", "user@example.com", ""); err != nil {
		t.Fatalf("second send: %v", err)
	}

	// Two rows, exactly one still consumable.
	if len(repo.tokens) != 2 {
		t.Fatalf("len(tokens) = %d, want 2", len(repo.tokens))
	}
	live := 0
	for _, tok := range repo.tokens {
		if tok.ConsumedAt == nil {
			live++
		}
	}
	if live != 1 {
		t.Errorf("live tokens = %d, want 1", live)
	}
}

// TestSendMagicLink_RejectsEmptyInputs covers the boundary validation.
func TestSendMagicLink_RejectsEmptyInputs(t *testing.T) {
	svc := newService(newFakeRepo(), &captureSender{})
	ctx := context.Background()

	if err := svc.SendMagicLink(ctx, "", "user@example.com", ""); err == nil {
		t.Error("expected error for empty session id")
	}
	if err := svc.SendMagicLink(ctx, "sess", "", ""); err == nil {
		t.Error("expected error for empty email")
	}
}

// TestVerifyToken_HappyPath: a valid token resolves to its (sessionID,
// email) tuple and is marked consumed in the repo.
func TestVerifyToken_HappyPath(t *testing.T) {
	repo := newFakeRepo()
	sender := &captureSender{}
	svc := newService(repo, sender)
	ctx := context.Background()

	if err := svc.SendMagicLink(ctx, "sess-1", "user@example.com", "Acme"); err != nil {
		t.Fatalf("send: %v", err)
	}
	token := extractToken(t, sender.last.HTMLBody)

	res, err := svc.VerifyToken(ctx, token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if res.SessionID != "sess-1" || res.Email != "user@example.com" {
		t.Errorf("res = %+v", res)
	}

	// Replay must fail.
	if _, err := svc.VerifyToken(ctx, token); err == nil {
		t.Error("expected replay to fail")
	} else {
		var ae *apperrors.AppError
		if !errors.As(err, &ae) || ae.Code != "token_consumed" {
			t.Errorf("err = %v, want token_consumed", err)
		}
	}
}

// TestVerifyToken_Expired: an expired token must be rejected with the
// token_expired AppError code, even though the row still exists.
func TestVerifyToken_Expired(t *testing.T) {
	repo := newFakeRepo()
	sender := &captureSender{}
	svc := newService(repo, sender)
	ctx := context.Background()

	if err := svc.SendMagicLink(ctx, "sess-1", "user@example.com", ""); err != nil {
		t.Fatalf("send: %v", err)
	}
	// Force expiry on the stored token.
	for _, tok := range repo.tokens {
		tok.ExpiresAt = time.Now().Add(-1 * time.Minute)
	}
	token := extractToken(t, sender.last.HTMLBody)

	_, err := svc.VerifyToken(ctx, token)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
	var ae *apperrors.AppError
	if !errors.As(err, &ae) || ae.Code != "token_expired" {
		t.Errorf("err = %v, want token_expired", err)
	}
}

// TestVerifyToken_Unknown: random tokens fail with NotFound.
func TestVerifyToken_Unknown(t *testing.T) {
	svc := newService(newFakeRepo(), &captureSender{})
	_, err := svc.VerifyToken(context.Background(), "not-a-real-token")
	if err == nil {
		t.Fatal("expected NotFound for unknown token")
	}
	var ae *apperrors.AppError
	if !errors.As(err, &ae) || ae.Status != 404 {
		t.Errorf("err = %v, want 404 NotFound", err)
	}
}

// TestVerifyToken_Empty: rejects empty input.
func TestVerifyToken_Empty(t *testing.T) {
	svc := newService(newFakeRepo(), &captureSender{})
	if _, err := svc.VerifyToken(context.Background(), ""); err == nil {
		t.Error("expected error for empty token")
	}
}

// TestGenerateToken: tokens are unique high-entropy strings whose hash
// is deterministic and matches hashToken.
func TestGenerateToken(t *testing.T) {
	tok1, hash1, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}
	tok2, hash2, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}
	if tok1 == tok2 || hash1 == hash2 {
		t.Error("generateToken produced duplicate output")
	}
	if hashToken(tok1) != hash1 {
		t.Error("hashToken not deterministic with generateToken")
	}
	if len(tok1) < 32 {
		t.Errorf("token too short: %d chars", len(tok1))
	}
}

// extractToken pulls the token query parameter out of a magic-link URL
// embedded in the rendered email body.
func extractToken(t *testing.T, body string) string {
	t.Helper()
	const marker = "token="
	idx := indexOf(body, marker)
	if idx < 0 {
		t.Fatalf("body missing %q: %s", marker, body)
	}
	rest := body[idx+len(marker):]
	end := len(rest)
	for i, r := range rest {
		if r == '"' || r == '<' || r == ' ' || r == '\n' || r == '&' {
			end = i
			break
		}
	}
	return rest[:end]
}

// Tiny helpers to keep the test file dependency-free.
func contains(haystack, needle string) bool { return indexOf(haystack, needle) >= 0 }
func indexOf(haystack, needle string) int {
	if len(needle) == 0 {
		return 0
	}
outer:
	for i := 0; i+len(needle) <= len(haystack); i++ {
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}
