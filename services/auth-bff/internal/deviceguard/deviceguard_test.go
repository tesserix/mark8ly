package deviceguard

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type memKnown struct {
	mu    sync.Mutex
	seen  map[string]bool
	err   error
	calls int
}

func newMemKnown() *memKnown { return &memKnown{seen: map[string]bool{}} }

func (m *memKnown) HasSeen(_ context.Context, userID, fingerprint string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.err != nil {
		return false, m.err
	}
	return m.seen[userID+"|"+fingerprint], nil
}

func (m *memKnown) markSeen(userID, fingerprint string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seen[userID+"|"+fingerprint] = true
}

type recordingNotifier struct {
	mu     sync.Mutex
	alerts []Alert
	err    error
}

func (r *recordingNotifier) NotifyNewDevice(_ context.Context, a Alert) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.alerts = append(r.alerts, a)
	return r.err
}

func (r *recordingNotifier) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.alerts)
}

func (r *recordingNotifier) last() Alert {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.alerts[len(r.alerts)-1]
}

func newTestService(t *testing.T, store *memKnown, n *recordingNotifier) *Service {
	t.Helper()
	svc, err := NewService(Config{Store: store, Notifier: n})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func TestFingerprintIsStableForSameClient(t *testing.T) {
	ua := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)"
	if Fingerprint(ua) != Fingerprint(ua) {
		t.Error("fingerprint is not stable across calls")
	}
}

func TestFingerprintDiffersBetweenClients(t *testing.T) {
	a := Fingerprint("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
	b := Fingerprint("Mozilla/5.0 (iPhone; CPU iPhone OS 17_0)")
	if a == b {
		t.Error("different user agents produced the same fingerprint")
	}
}

func TestFingerprintIsNotReversible(t *testing.T) {
	ua := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)"
	fp := Fingerprint(ua)
	if len(fp) != 64 {
		t.Errorf("fingerprint length = %d, want 64 hex chars", len(fp))
	}
	if fp == ua {
		t.Error("fingerprint is the raw user agent")
	}
}

func TestFingerprintOfEmptyUserAgentIsStillDeterministic(t *testing.T) {
	if Fingerprint("") == "" {
		t.Error("empty user agent should still hash to a value")
	}
	if Fingerprint("") != Fingerprint("") {
		t.Error("empty user agent fingerprint is unstable")
	}
}

func TestUnknownDeviceRaisesAlert(t *testing.T) {
	store, notifier := newMemKnown(), &recordingNotifier{}
	svc := newTestService(t, store, notifier)

	isNew, err := svc.Evaluate(context.Background(), Login{
		UserID:      "uid-1",
		Email:       "user@example.com",
		Fingerprint: "fp-1",
		Device:      "Mac",
		IPAddress:   "203.0.113.9",
		Country:     "IN",
		At:          time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !isNew {
		t.Error("first login from a device should be reported as new")
	}
	if notifier.count() != 1 {
		t.Fatalf("want 1 alert, got %d", notifier.count())
	}
	got := notifier.last()
	if got.Email != "user@example.com" || got.Device != "Mac" || got.Country != "IN" {
		t.Errorf("alert carried wrong details: %+v", got)
	}
}

func TestKnownDeviceRaisesNoAlert(t *testing.T) {
	store, notifier := newMemKnown(), &recordingNotifier{}
	store.markSeen("uid-1", "fp-1")
	svc := newTestService(t, store, notifier)

	isNew, err := svc.Evaluate(context.Background(), Login{
		UserID: "uid-1", Email: "user@example.com", Fingerprint: "fp-1",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if isNew {
		t.Error("known device reported as new")
	}
	if notifier.count() != 0 {
		t.Errorf("want no alert for a known device, got %d", notifier.count())
	}
}

func TestAlertIsPerUserNotGlobal(t *testing.T) {
	store, notifier := newMemKnown(), &recordingNotifier{}
	store.markSeen("uid-1", "fp-shared")
	svc := newTestService(t, store, notifier)

	// Same browser fingerprint, different account — still a new device
	// for that account and must alert.
	isNew, err := svc.Evaluate(context.Background(), Login{
		UserID: "uid-2", Email: "other@example.com", Fingerprint: "fp-shared",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !isNew {
		t.Error("shared fingerprint on a different user should count as new")
	}
	if notifier.count() != 1 {
		t.Errorf("want 1 alert, got %d", notifier.count())
	}
}

func TestUnknownCountryIsDescribedNotBlank(t *testing.T) {
	store, notifier := newMemKnown(), &recordingNotifier{}
	svc := newTestService(t, store, notifier)

	if _, err := svc.Evaluate(context.Background(), Login{
		UserID: "uid-1", Email: "user@example.com", Fingerprint: "fp-1", Country: "",
	}); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got := notifier.last().CountryName; got != "an unknown location" {
		t.Errorf("CountryName = %q, want a human fallback", got)
	}
}

func TestCountryNameIsResolvedFromCode(t *testing.T) {
	store, notifier := newMemKnown(), &recordingNotifier{}
	svc := newTestService(t, store, notifier)

	svc.Evaluate(context.Background(), Login{
		UserID: "uid-1", Email: "user@example.com", Fingerprint: "fp-1", Country: "IN",
	})
	if got := notifier.last().CountryName; got != "India" {
		t.Errorf("CountryName = %q, want %q", got, "India")
	}
}

// A login must succeed even when the alerting path is broken — a failed
// notification is not a reason to deny a legitimate user.
func TestNotifierFailureDoesNotFailLogin(t *testing.T) {
	store := newMemKnown()
	notifier := &recordingNotifier{err: errors.New("sendgrid down")}
	svc := newTestService(t, store, notifier)

	isNew, err := svc.Evaluate(context.Background(), Login{
		UserID: "uid-1", Email: "user@example.com", Fingerprint: "fp-1",
	})
	if err != nil {
		t.Fatalf("notifier failure leaked to caller: %v", err)
	}
	if !isNew {
		t.Error("device should still be reported as new")
	}
}

func TestStoreFailureFailsClosedWithoutBlockingLogin(t *testing.T) {
	store := newMemKnown()
	store.err = errors.New("db down")
	notifier := &recordingNotifier{}
	svc := newTestService(t, store, notifier)

	isNew, err := svc.Evaluate(context.Background(), Login{
		UserID: "uid-1", Email: "user@example.com", Fingerprint: "fp-1",
	})
	if err != nil {
		t.Fatalf("store failure leaked to caller: %v", err)
	}
	// Unknown state must be treated as suspicious and alert, never silently pass.
	if !isNew {
		t.Error("store failure should be treated as an unrecognised device")
	}
	if notifier.count() != 1 {
		t.Errorf("want an alert when device history is unavailable, got %d", notifier.count())
	}
}

func TestEmptyFingerprintIsTreatedAsNewDevice(t *testing.T) {
	store, notifier := newMemKnown(), &recordingNotifier{}
	store.markSeen("uid-1", "")
	svc := newTestService(t, store, notifier)

	isNew, _ := svc.Evaluate(context.Background(), Login{
		UserID: "uid-1", Email: "user@example.com", Fingerprint: "",
	})
	if !isNew {
		t.Error("a missing fingerprint must not be matchable against history")
	}
	if notifier.count() != 1 {
		t.Errorf("want 1 alert, got %d", notifier.count())
	}
}

func TestNoNotifierConfiguredIsSafe(t *testing.T) {
	svc, err := NewService(Config{Store: newMemKnown()})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if _, err := svc.Evaluate(context.Background(), Login{
		UserID: "uid-1", Email: "user@example.com", Fingerprint: "fp-1",
	}); err != nil {
		t.Fatalf("Evaluate without notifier: %v", err)
	}
}
