package trial

// expiry_email_test.go — pure unit tests for the trial-expired notice.
//
// These deliberately need no database. Every other test in this package is
// an integration test gated on TEST_DATABASE_URL, which means it skips
// silently when the DSN is absent; the email decision is small enough to
// exercise directly, so it is tested where it always runs.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/statemachine"
)

type recordedSend struct {
	template email.TemplateID
	to       string
	data     map[string]any
}

type fakeEmailClient struct {
	sends []recordedSend
	err   error
}

func (f *fakeEmailClient) Send(_ context.Context, template email.TemplateID, to string, data map[string]any) error {
	f.sends = append(f.sends, recordedSend{template: template, to: to, data: data})
	return f.err
}

type stubCounter struct{ n *int }

func (s stubCounter) Inc() { *s.n++ }

type stubSentCounter struct {
	labels []string
	n      int
}

func (s *stubSentCounter) WithTemplate(template string) CounterIncrementer {
	s.labels = append(s.labels, template)
	return stubCounter{n: &s.n}
}

type stubSkipCounter struct {
	labels [][2]string
	n      int
}

func (s *stubSkipCounter) WithTemplateReason(template, reason string) CounterIncrementer {
	s.labels = append(s.labels, [2]string{template, reason})
	return stubCounter{n: &s.n}
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func expiredRow(addr *string) *subscription.StoreSubscription {
	return &subscription.StoreSubscription{
		TenantID: uuid.New(),
		StoreID:  uuid.New(),
		Email:    addr,
	}
}

func strptr(s string) *string { return &s }

func TestAfterTransition_SendsTrialExpiredOnSuccess(t *testing.T) {
	mailer := &fakeEmailClient{}
	sent := &stubSentCounter{}
	skipped := &stubSkipCounter{}
	cron := NewExpiryCron(nil, nil, quietLogger(), nil).WithEmail(mailer, sent, skipped)

	row := expiredRow(strptr("merchant@example.com"))
	cron.afterTransition(context.Background(), row, nil)

	if len(mailer.sends) != 1 {
		t.Fatalf("sends = %d, want exactly 1", len(mailer.sends))
	}
	got := mailer.sends[0]
	if got.template != email.TemplateTrialExpired {
		t.Errorf("template = %q, want %q", got.template, email.TemplateTrialExpired)
	}
	if got.to != "merchant@example.com" {
		t.Errorf("to = %q, want merchant@example.com", got.to)
	}
	if _, ok := got.data["store_name"]; !ok {
		t.Error("data is missing store_name")
	}
	if got.data["store_id"] != row.StoreID.String() {
		t.Errorf("data store_id = %v, want %s", got.data["store_id"], row.StoreID)
	}
	if sent.n != 1 {
		t.Errorf("sent counter = %d, want 1", sent.n)
	}
	if skipped.n != 0 {
		t.Errorf("skip counter = %d, want 0", skipped.n)
	}
}

func TestAfterTransition_NoMailOnCASConflict(t *testing.T) {
	mailer := &fakeEmailClient{}
	cron := NewExpiryCron(nil, nil, quietLogger(), nil).WithEmail(mailer, nil, nil)

	cron.afterTransition(context.Background(), expiredRow(strptr("merchant@example.com")),
		statemachine.ErrCASConflict)

	if len(mailer.sends) != 0 {
		t.Fatalf("sends = %d, want 0 — the store did not expire", len(mailer.sends))
	}
}

func TestAfterTransition_NoMailOnTransitionFailure(t *testing.T) {
	mailer := &fakeEmailClient{}
	cron := NewExpiryCron(nil, nil, quietLogger(), nil).WithEmail(mailer, nil, nil)

	cron.afterTransition(context.Background(), expiredRow(strptr("merchant@example.com")),
		errors.New("boom"))

	if len(mailer.sends) != 0 {
		t.Fatalf("sends = %d, want 0", len(mailer.sends))
	}
}

func TestAfterTransition_NilEmailClientIsANoOp(t *testing.T) {
	cron := NewExpiryCron(nil, nil, quietLogger(), nil)

	// Must not panic: trial expiry never depends on email being configured.
	cron.afterTransition(context.Background(), expiredRow(strptr("merchant@example.com")), nil)
}

func TestAfterTransition_MissingRecipientIsNotSent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		addr   *string
		reason string
	}{
		{"nil email", nil, email.ReasonNoAddress},
		{"empty email", strptr(""), email.ReasonNoAddress},
		{"placeholder email", strptr("billing+x@mark8ly.local"), email.ReasonPlaceholderAddress},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mailer := &fakeEmailClient{}
			skipped := &stubSkipCounter{}
			sent := &stubSentCounter{}
			cron := NewExpiryCron(nil, nil, quietLogger(), nil).WithEmail(mailer, sent, skipped)

			cron.afterTransition(context.Background(), expiredRow(tc.addr), nil)

			if len(mailer.sends) != 0 {
				t.Fatalf("sends = %d, want 0 — no deliverable address", len(mailer.sends))
			}
			if sent.n != 0 {
				t.Errorf("sent counter = %d, want 0", sent.n)
			}
			if skipped.n != 1 {
				t.Fatalf("skip counter = %d, want 1", skipped.n)
			}
			want := [2]string{string(email.TemplateTrialExpired), tc.reason}
			if skipped.labels[0] != want {
				t.Errorf("skip labels = %v, want %v", skipped.labels[0], want)
			}
		})
	}
}

func TestAfterTransition_SendFailureIsCountedAsSkipped(t *testing.T) {
	mailer := &fakeEmailClient{err: errors.New("transport down")}
	sent := &stubSentCounter{}
	skipped := &stubSkipCounter{}
	cron := NewExpiryCron(nil, nil, quietLogger(), nil).WithEmail(mailer, sent, skipped)

	cron.afterTransition(context.Background(), expiredRow(strptr("merchant@example.com")), nil)

	if len(mailer.sends) != 1 {
		t.Fatalf("sends = %d, want 1 attempt", len(mailer.sends))
	}
	if sent.n != 0 {
		t.Errorf("sent counter = %d, want 0 — the send failed", sent.n)
	}
	if skipped.n != 1 {
		t.Errorf("skip counter = %d, want 1", skipped.n)
	}
}
