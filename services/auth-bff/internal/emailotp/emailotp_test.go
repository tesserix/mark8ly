package emailotp

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

const testPepper = "0123456789abcdef0123456789abcdef"

// memStore is an in-memory Store used to exercise Service behaviour
// without a database. It mirrors the ordering guarantees of the
// Postgres implementation: Latest returns the newest row per email.
type memStore struct {
	mu   sync.Mutex
	recs []Record
	err  error
}

func (m *memStore) Insert(_ context.Context, r Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.recs = append(m.recs, r)
	return nil
}

func (m *memStore) Latest(_ context.Context, email string) (*Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.recs) - 1; i >= 0; i-- {
		if m.recs[i].Email == email {
			cp := m.recs[i]
			return &cp, nil
		}
	}
	return nil, ErrNoChallenge
}

func (m *memStore) IncrementAttempts(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.recs {
		if m.recs[i].ID == id {
			m.recs[i].Attempts++
			return nil
		}
	}
	return ErrNoChallenge
}

func (m *memStore) Consume(_ context.Context, id string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.recs {
		if m.recs[i].ID == id {
			if m.recs[i].ConsumedAt != nil {
				return ErrNoChallenge
			}
			m.recs[i].ConsumedAt = &at
			return nil
		}
	}
	return ErrNoChallenge
}

func (m *memStore) CountSince(_ context.Context, email string, since time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, r := range m.recs {
		if r.Email == email && r.CreatedAt.After(since) {
			n++
		}
	}
	return n, nil
}

// fixedClock lets tests advance time deliberately instead of sleeping.
type fixedClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fixedClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fixedClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestService(t *testing.T) (*Service, *memStore, *fixedClock) {
	t.Helper()
	store := &memStore{}
	clk := &fixedClock{t: time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)}
	svc, err := NewService(Config{
		Store:  store,
		Pepper: testPepper,
		Now:    clk.now,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, store, clk
}

func TestNewServiceRejectsShortPepper(t *testing.T) {
	_, err := NewService(Config{Store: &memStore{}, Pepper: "tooshort"})
	if !errors.Is(err, ErrWeakPepper) {
		t.Fatalf("want ErrWeakPepper, got %v", err)
	}
}

func TestNewServiceRequiresStore(t *testing.T) {
	_, err := NewService(Config{Pepper: testPepper})
	if err == nil {
		t.Fatal("want error when store is nil")
	}
}

func TestRequestReturnsSixDigitCode(t *testing.T) {
	svc, _, _ := newTestService(t)

	ch, err := svc.Request(context.Background(), "user@example.com", "1.2.3.4")
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if !regexp.MustCompile(`^[0-9]{6}$`).MatchString(ch.Code) {
		t.Errorf("code = %q, want six digits", ch.Code)
	}
}

func TestRequestNeverStoresPlaintextCode(t *testing.T) {
	svc, store, _ := newTestService(t)

	ch, err := svc.Request(context.Background(), "user@example.com", "1.2.3.4")
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if len(store.recs) != 1 {
		t.Fatalf("want 1 stored record, got %d", len(store.recs))
	}
	if strings.Contains(string(store.recs[0].CodeHash), ch.Code) {
		t.Error("stored hash contains the plaintext code")
	}
	if len(store.recs[0].CodeHash) != 32 {
		t.Errorf("hash length = %d, want 32 (sha256)", len(store.recs[0].CodeHash))
	}
}

func TestRequestProducesVaryingCodes(t *testing.T) {
	// The rate limiter would cut this off at 5, so this case gets its own
	// budget — it is exercising the code generator, not the limiter.
	svc, err := NewService(Config{
		Store:        &memStore{},
		Pepper:       testPepper,
		MaxPerWindow: 1000,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	seen := map[string]bool{}
	for i := 0; i < 40; i++ {
		ch, err := svc.Request(context.Background(), "user@example.com", "1.2.3.4")
		if err != nil {
			t.Fatalf("Request %d: %v", i, err)
		}
		seen[ch.Code] = true
	}
	// 40 draws from 10^6 colliding into under 5 buckets would mean the
	// generator is broken, not unlucky.
	if len(seen) < 5 {
		t.Errorf("only %d distinct codes in 40 requests — generator looks broken", len(seen))
	}
}

func TestRequestNormalisesEmail(t *testing.T) {
	svc, store, _ := newTestService(t)

	if _, err := svc.Request(context.Background(), "  User@Example.COM ", "1.2.3.4"); err != nil {
		t.Fatalf("Request: %v", err)
	}
	if got := store.recs[0].Email; got != "user@example.com" {
		t.Errorf("stored email = %q, want normalised", got)
	}
}

func TestVerifyAcceptsCorrectCode(t *testing.T) {
	svc, _, _ := newTestService(t)

	ch, _ := svc.Request(context.Background(), "user@example.com", "1.2.3.4")
	if err := svc.Verify(context.Background(), "user@example.com", ch.Code); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerifyIsCaseAndSpaceTolerantOnEmail(t *testing.T) {
	svc, _, _ := newTestService(t)

	ch, _ := svc.Request(context.Background(), "user@example.com", "1.2.3.4")
	if err := svc.Verify(context.Background(), " USER@example.com ", " "+ch.Code+" "); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerifyRejectsWrongCode(t *testing.T) {
	svc, _, _ := newTestService(t)

	ch, _ := svc.Request(context.Background(), "user@example.com", "1.2.3.4")
	wrong := "000000"
	if ch.Code == wrong {
		wrong = "111111"
	}
	if err := svc.Verify(context.Background(), "user@example.com", wrong); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("want ErrInvalidCode, got %v", err)
	}
}

func TestVerifyRejectsCodeIssuedForAnotherEmail(t *testing.T) {
	svc, _, _ := newTestService(t)

	ch, _ := svc.Request(context.Background(), "victim@example.com", "1.2.3.4")
	if _, err := svc.Request(context.Background(), "attacker@example.com", "1.2.3.4"); err != nil {
		t.Fatalf("Request: %v", err)
	}
	// The victim's code must not validate against the attacker's challenge,
	// even if the digits happen to match — the email is bound into the hash.
	err := svc.Verify(context.Background(), "attacker@example.com", ch.Code)
	if err == nil {
		t.Fatal("victim code validated against attacker challenge")
	}
}

func TestVerifyRejectsExpiredCode(t *testing.T) {
	svc, _, clk := newTestService(t)

	ch, _ := svc.Request(context.Background(), "user@example.com", "1.2.3.4")
	clk.advance(DefaultTTL + time.Second)

	if err := svc.Verify(context.Background(), "user@example.com", ch.Code); !errors.Is(err, ErrExpired) {
		t.Fatalf("want ErrExpired, got %v", err)
	}
}

func TestVerifyAcceptsCodeJustInsideTTL(t *testing.T) {
	svc, _, clk := newTestService(t)

	ch, _ := svc.Request(context.Background(), "user@example.com", "1.2.3.4")
	clk.advance(DefaultTTL - time.Second)

	if err := svc.Verify(context.Background(), "user@example.com", ch.Code); err != nil {
		t.Fatalf("Verify at TTL boundary: %v", err)
	}
}

func TestVerifyIsSingleUse(t *testing.T) {
	svc, _, _ := newTestService(t)

	ch, _ := svc.Request(context.Background(), "user@example.com", "1.2.3.4")
	if err := svc.Verify(context.Background(), "user@example.com", ch.Code); err != nil {
		t.Fatalf("first Verify: %v", err)
	}
	err := svc.Verify(context.Background(), "user@example.com", ch.Code)
	if !errors.Is(err, ErrAlreadyUsed) {
		t.Fatalf("replay: want ErrAlreadyUsed, got %v", err)
	}
}

func TestVerifyLocksOutAfterMaxAttempts(t *testing.T) {
	svc, _, _ := newTestService(t)

	ch, _ := svc.Request(context.Background(), "user@example.com", "1.2.3.4")
	for i := 0; i < DefaultMaxAttempts; i++ {
		if err := svc.Verify(context.Background(), "user@example.com", "999999"); errors.Is(err, ErrTooManyAttempts) {
			t.Fatalf("locked out early at attempt %d", i+1)
		}
	}
	// The correct code must no longer work once the budget is spent.
	if err := svc.Verify(context.Background(), "user@example.com", ch.Code); !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("want ErrTooManyAttempts, got %v", err)
	}
}

func TestVerifyWithNoChallengeIsRejected(t *testing.T) {
	svc, _, _ := newTestService(t)

	err := svc.Verify(context.Background(), "nobody@example.com", "123456")
	if !errors.Is(err, ErrNoChallenge) {
		t.Fatalf("want ErrNoChallenge, got %v", err)
	}
}

func TestVerifyRejectsMalformedCodeWithoutTouchingStore(t *testing.T) {
	svc, store, _ := newTestService(t)
	svc.Request(context.Background(), "user@example.com", "1.2.3.4")
	before := store.recs[0].Attempts

	for _, bad := range []string{"", "12345", "1234567", "abcdef", "12 34 5"} {
		if err := svc.Verify(context.Background(), "user@example.com", bad); !errors.Is(err, ErrInvalidCode) {
			t.Errorf("Verify(%q): want ErrInvalidCode, got %v", bad, err)
		}
	}
	// Malformed input must not burn the user's attempt budget.
	if store.recs[0].Attempts != before {
		t.Errorf("attempts moved from %d to %d on malformed input", before, store.recs[0].Attempts)
	}
}

func TestRequestRateLimitsPerEmail(t *testing.T) {
	svc, _, _ := newTestService(t)

	for i := 0; i < DefaultMaxPerWindow; i++ {
		if _, err := svc.Request(context.Background(), "user@example.com", "1.2.3.4"); err != nil {
			t.Fatalf("Request %d: %v", i, err)
		}
	}
	if _, err := svc.Request(context.Background(), "user@example.com", "1.2.3.4"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("want ErrRateLimited, got %v", err)
	}
}

func TestRateLimitWindowRollsOff(t *testing.T) {
	svc, _, clk := newTestService(t)

	for i := 0; i < DefaultMaxPerWindow; i++ {
		svc.Request(context.Background(), "user@example.com", "1.2.3.4")
	}
	clk.advance(DefaultRateWindow + time.Second)

	if _, err := svc.Request(context.Background(), "user@example.com", "1.2.3.4"); err != nil {
		t.Fatalf("after window roll-off: %v", err)
	}
}

func TestRateLimitIsPerEmail(t *testing.T) {
	svc, _, _ := newTestService(t)

	for i := 0; i < DefaultMaxPerWindow; i++ {
		svc.Request(context.Background(), "a@example.com", "1.2.3.4")
	}
	if _, err := svc.Request(context.Background(), "b@example.com", "1.2.3.4"); err != nil {
		t.Fatalf("unrelated email should not be limited: %v", err)
	}
}

func TestRequestRejectsEmptyEmail(t *testing.T) {
	svc, _, _ := newTestService(t)

	if _, err := svc.Request(context.Background(), "   ", "1.2.3.4"); !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("want ErrInvalidEmail, got %v", err)
	}
}

func TestConcurrentRequestsAreSafe(t *testing.T) {
	svc, _, _ := newTestService(t)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			svc.Request(context.Background(), string(rune('a'+n))+"@example.com", "1.2.3.4")
		}(i)
	}
	wg.Wait()
}
