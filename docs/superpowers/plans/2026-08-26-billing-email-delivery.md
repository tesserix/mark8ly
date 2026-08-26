# Billing Email Delivery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make dunning, trial-reminder, payment-action, win-back and trial-billed emails actually reach merchants, with counters that cannot overstate delivery.

**Architecture:** A single new `email.Client` implementation renders a key through the existing `emailtemplates.Loader` and delegates to the existing SendGrid→Resend `Sender` chain. Callers resolve the recipient from a new `store_subscriptions.email` column (they already scan that table) and pass a real address. The adapter refuses undeliverable addresses with a typed sentinel instead of returning `nil`, which is what makes the existing counters honest. Two unguarded crons gain claim-first idempotency markers.

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL (golang-migrate), Prometheus client_golang, Stripe Go SDK v82, `html/template` + `text/template`.

**Spec:** `docs/superpowers/specs/2026-08-26-billing-email-delivery-design.md`

## Global Constraints

- Branch `fix/381-billing-email-delivery`, worktree `/tmp/m8-381`. **Never** push, open a PR, merge or deploy.
- Module path is `github.com/mark8ly/marketplace-api`; all work is under `services/marketplace-api/`.
- Integration tests are build-tagged. `go vet -tags=integration ./...` is the only thing that compiles them. Run integration suites with `-p 1`.
- `TEST_DATABASE_URL=postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable` — LAN IP `192.168.1.110`, never `localhost`. The dev Postgres is container `dev-postgres-1`; if `5432` is refused, `docker start dev-postgres-1`.
- Before starting any integration run, check nothing else is running one: `ps aux | grep "go test -tags=integration"`.
- **There are 22 packages / 191 tests already failing at baseline.** Never call a failure pre-existing without diffing against a throwaway worktree of the base commit, in both directions.
- Migration files are pairs: `migrations/000NNN_<name>.up.sql` and `.down.sql`. Next free number is **000104**. Every `.up.sql` and `.down.sql` opens with a comment explaining intent and, for `.down.sql`, what data is lost.
- Commit messages are single-line conventional commits. No signatures, no `Co-Authored-By`, no multi-line bodies.
- Template keys are the existing `email.TemplateID` string values. Do not invent a second identifier.

---

### Task 1: Undeliverable-recipient sentinel

The rule that makes every counter in this plan honest: the adapter must never return `nil` for mail it did not send. This task builds the classifier; Task 3 wires it in.

**Files:**
- Create: `internal/email/recipient.go`
- Test: `internal/email/recipient_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `var ErrUndeliverable error`
  - `type UndeliverableError struct{ Reason string }` with `Error() string` and `Unwrap() error`
  - `func ValidateRecipient(to string) error` — returns `nil` or `*UndeliverableError`
  - `func UndeliverableReason(err error) (string, bool)`
  - `const ReasonNoAddress = "no_address"`, `ReasonInvalidAddress = "invalid_address"`, `ReasonPlaceholderAddress = "placeholder_address"`

- [ ] **Step 1: Write the failing test**

Create `internal/email/recipient_test.go`:

```go
package email_test

import (
	"errors"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/email"
)

func TestValidateRecipient(t *testing.T) {
	cases := []struct {
		name       string
		to         string
		wantReason string // "" means deliverable
	}{
		{"plain address", "merchant@example.com", ""},
		{"address with display parts trimmed", "  merchant@example.com  ", ""},
		{"subaddressed", "billing+store@example.com", ""},
		{"empty", "", email.ReasonNoAddress},
		{"whitespace only", "   ", email.ReasonNoAddress},
		{"no at sign", "merchant.example.com", email.ReasonInvalidAddress},
		{"two at signs", "a@b@example.com", email.ReasonInvalidAddress},
		{"empty local part", "@example.com", email.ReasonInvalidAddress},
		{"empty domain", "merchant@", email.ReasonInvalidAddress},
		{"domain without dot", "merchant@localhost", email.ReasonPlaceholderAddress},
		{"bootstrap placeholder", "billing+7f3a@mark8ly.local", email.ReasonPlaceholderAddress},
		{"uppercase placeholder", "ops@MARK8LY.LOCAL", email.ReasonPlaceholderAddress},
		{"rfc2606 invalid", "ops@something.invalid", email.ReasonPlaceholderAddress},
		{"rfc2606 test", "ops@something.test", email.ReasonPlaceholderAddress},
		{"rfc2606 example", "ops@something.example", email.ReasonPlaceholderAddress},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := email.ValidateRecipient(tc.to)
			if tc.wantReason == "" {
				if err != nil {
					t.Fatalf("ValidateRecipient(%q) = %v, want nil", tc.to, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateRecipient(%q) = nil, want %s", tc.to, tc.wantReason)
			}
			if !errors.Is(err, email.ErrUndeliverable) {
				t.Errorf("errors.Is(err, ErrUndeliverable) = false for %q", tc.to)
			}
			reason, ok := email.UndeliverableReason(err)
			if !ok {
				t.Fatalf("UndeliverableReason(%v) not recognised", err)
			}
			if reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", reason, tc.wantReason)
			}
		})
	}
}

func TestUndeliverableReason_UnrelatedError(t *testing.T) {
	if _, ok := email.UndeliverableReason(errors.New("boom")); ok {
		t.Error("unrelated error reported as undeliverable")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /tmp/m8-381/services/marketplace-api && go test ./internal/email/ -run TestValidateRecipient -v
```

Expected: FAIL — build error, `undefined: email.ValidateRecipient`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/email/recipient.go`:

```go
package email

// recipient.go — the guard that keeps billing mail out of the bit bucket
// and keeps the delivery counters honest.
//
// subscription/service.go mints billing+<store_id>@mark8ly.local whenever a
// subscription is bootstrapped without a real email. `.local` is unroutable,
// so sending there hard-bounces and costs sender reputation. Rather than
// discover that at the provider, we classify the address up front and return
// a typed error — never nil — so a caller can count the skip instead of
// recording a delivery that never happened.

import (
	"errors"
	"fmt"
	"strings"
)

// ErrUndeliverable marks a recipient we refuse to attempt delivery to.
// Wrapped, so callers can errors.Is regardless of reason.
var ErrUndeliverable = errors.New("undeliverable recipient")

// Reasons an address is refused. These become the `reason` label on
// mark8ly_subscription_billing_emails_skipped_total, so keep them stable.
const (
	ReasonNoAddress          = "no_address"
	ReasonInvalidAddress     = "invalid_address"
	ReasonPlaceholderAddress = "placeholder_address"
)

// placeholderSuffixes are domains that can never receive mail: the RFC 2606
// reserved TLDs plus bare `localhost`. `.local` catches the
// billing+<uuid>@mark8ly.local addresses minted at bootstrap.
var placeholderSuffixes = []string{
	".local",
	".invalid",
	".test",
	".example",
	"localhost",
}

// UndeliverableError carries why an address was refused.
type UndeliverableError struct{ Reason string }

func (e *UndeliverableError) Error() string {
	return fmt.Sprintf("email: %s: %s", ErrUndeliverable.Error(), e.Reason)
}

// Unwrap lets errors.Is(err, ErrUndeliverable) succeed.
func (e *UndeliverableError) Unwrap() error { return ErrUndeliverable }

// ValidateRecipient reports whether `to` is worth handing to a provider.
// Deliberately conservative: this is a bounce guard, not an RFC 5321 parser.
func ValidateRecipient(to string) error {
	addr := strings.TrimSpace(to)
	if addr == "" {
		return &UndeliverableError{Reason: ReasonNoAddress}
	}

	local, domain, found := strings.Cut(addr, "@")
	if !found || local == "" || domain == "" || strings.Contains(domain, "@") {
		return &UndeliverableError{Reason: ReasonInvalidAddress}
	}

	lower := strings.ToLower(domain)
	for _, suffix := range placeholderSuffixes {
		if lower == suffix || strings.HasSuffix(lower, suffix) {
			return &UndeliverableError{Reason: ReasonPlaceholderAddress}
		}
	}
	if !strings.Contains(lower, ".") {
		// No dot at all — not a routable public domain.
		return &UndeliverableError{Reason: ReasonPlaceholderAddress}
	}
	return nil
}

// UndeliverableReason extracts the reason from an error produced by
// ValidateRecipient, reporting false for anything else (e.g. a transport
// failure, which must be counted differently).
func UndeliverableReason(err error) (string, bool) {
	var ue *UndeliverableError
	if errors.As(err, &ue) {
		return ue.Reason, true
	}
	return "", false
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /tmp/m8-381/services/marketplace-api && go test ./internal/email/ -run 'TestValidateRecipient|TestUndeliverableReason' -v
```

Expected: PASS, all subtests.

- [ ] **Step 5: Commit**

```bash
cd /tmp/m8-381 && git add services/marketplace-api/internal/email/recipient.go services/marketplace-api/internal/email/recipient_test.go
git commit -m "feat(email): classify undeliverable recipients with a typed sentinel"
```

---

### Task 2: Skipped-delivery metric

Counters that record *not* sending. Without this the `.local` population is invisible rather than merely uncounted.

**Files:**
- Modify: `internal/metrics/registry.go` (add to the `var` block near `DunningEmailsSentTotal` at :79-87, and to the `MustRegister` list at :168-182)
- Modify: `internal/subscription/dunning/metrics_adapter.go`
- Test: `internal/subscription/dunning/metrics_adapter_test.go` — **this file already exists**; append to it, do not recreate it.

**Interfaces:**
- Consumes: `dunning.CounterIncrementer` (exists, has `Inc()`).
- Produces:
  - `metrics.BillingEmailsSkippedTotal *prometheus.CounterVec` with labels `{"template","reason"}`
  - `dunning.SkipCounter` interface: `WithTemplateReason(template, reason string) CounterIncrementer`
  - `dunning.WrapPrometheusSkipCounter(cv *prometheus.CounterVec) SkipCounter`

- [ ] **Step 1: Write the failing test**

Append **only the test function** to the existing
`internal/subscription/dunning/metrics_adapter_test.go`. Do **not** add a
package clause and do **not** add an import block: the file is already
`package dunning_test` and already imports `prometheus`, `prometheus/testutil`
and `dunning` at lines 3-11. A second import of the same package in one file
is a compile error.

```go
// --- appended for #381 ---

func TestWrapPrometheusSkipCounter_IncrementsLabelledSeries(t *testing.T) {
	cv := prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "test_skipped_total", Help: "test"},
		[]string{"template", "reason"},
	)

	sc := dunning.WrapPrometheusSkipCounter(cv)
	sc.WithTemplateReason("dunning_day_5", "placeholder_address").Inc()
	sc.WithTemplateReason("dunning_day_5", "placeholder_address").Inc()
	sc.WithTemplateReason("dunning_day_7", "no_address").Inc()

	if got := testutil.ToFloat64(cv.WithLabelValues("dunning_day_5", "placeholder_address")); got != 2 {
		t.Errorf("day_5/placeholder = %v, want 2", got)
	}
	if got := testutil.ToFloat64(cv.WithLabelValues("dunning_day_7", "no_address")); got != 1 {
		t.Errorf("day_7/no_address = %v, want 1", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /tmp/m8-381/services/marketplace-api && go test ./internal/subscription/dunning/ -run TestWrapPrometheusSkipCounter -v
```

Expected: FAIL — `undefined: dunning.WrapPrometheusSkipCounter`.

- [ ] **Step 3: Write minimal implementation**

Append to `internal/subscription/dunning/metrics_adapter.go`:

```go
// SkipCounter counts billing emails that were deliberately NOT sent,
// labeled by template and reason. Defined here at the point of use rather
// than in the metrics package so the crons stay testable with a stub.
type SkipCounter interface {
	WithTemplateReason(template, reason string) CounterIncrementer
}

// WrapPrometheusSkipCounter adapts a two-label *prometheus.CounterVec to
// SkipCounter. Use with metrics.BillingEmailsSkippedTotal in main.go.
func WrapPrometheusSkipCounter(cv *prometheus.CounterVec) SkipCounter {
	return &prometheusSkipCounter{cv: cv}
}

type prometheusSkipCounter struct{ cv *prometheus.CounterVec }

func (p *prometheusSkipCounter) WithTemplateReason(template, reason string) CounterIncrementer {
	return prometheusCounter{c: p.cv.WithLabelValues(template, reason)}
}
```

In `internal/metrics/registry.go`, add after the `DunningEmailsSentTotal` block (ends at :87):

```go
	// BillingEmailsSkippedTotal counts subscription emails deliberately not
	// sent — an undeliverable recipient, a render failure, or a transport
	// failure. Its companion *_sent_total counters only ever increment on a
	// real delivery, so sent+skipped is the eligible population. #381.
	BillingEmailsSkippedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mark8ly_subscription_billing_emails_skipped_total",
			Help: "Count of subscription emails not sent, labeled by template and reason (no_address, placeholder_address, invalid_address, render_failed, transport_failed).",
		},
		[]string{"template", "reason"},
	)
```

And add `BillingEmailsSkippedTotal,` to the `MustRegister(...)` list, immediately after `TrialRemindersSentTotal,`.

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /tmp/m8-381/services/marketplace-api && go test ./internal/subscription/dunning/ -run TestWrapPrometheusSkipCounter -v && go build ./...
```

Expected: PASS, and the build succeeds.

- [ ] **Step 5: Commit**

```bash
cd /tmp/m8-381 && git add services/marketplace-api/internal/metrics/registry.go services/marketplace-api/internal/subscription/dunning/metrics_adapter.go services/marketplace-api/internal/subscription/dunning/metrics_adapter_test.go
git commit -m "feat(metrics): count billing emails skipped by template and reason"
```

---

### Task 3: The template client — the only real `email.Client`

**Files:**
- Create: `internal/email/template_client.go`
- Test: `internal/email/template_client_test.go`

**Interfaces:**
- Consumes: `ValidateRecipient`, `UndeliverableReason`, `ErrUndeliverable` (Task 1); `emailtemplates.Loader.Render`, `emailtemplates.EmbeddedFallback`, `Loader.Register` (existing); `email.Sender`, `email.Message` (existing).
- Produces:
  - `func NewTemplateClient(loader *emailtemplates.Loader, sender Sender, from string, logger *slog.Logger) Client`
  - `var ErrRender error`, `var ErrTransport error`
  - `const ReasonRenderFailed = "render_failed"`, `ReasonTransportFailed = "transport_failed"`, `ReasonUnknown = "unknown"`
  - `func SkipReason(err error) string`

There is no import cycle: `internal/emailtemplates` does not import `internal/email`.

- [ ] **Step 1: Write the failing test**

Create `internal/email/template_client_test.go`:

```go
package email_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
)

// captureSender records every Message handed to it.
type captureSender struct {
	mu   sync.Mutex
	msgs []email.Message
	err  error
}

func (s *captureSender) Send(_ context.Context, msg email.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.msgs = append(s.msgs, msg)
	return nil
}

func (s *captureSender) last(t *testing.T) email.Message {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.msgs) == 0 {
		t.Fatal("no message sent")
	}
	return s.msgs[len(s.msgs)-1]
}

// loaderWith returns a DB-less loader with one key registered.
func loaderWith(key, subject, html, text string) *emailtemplates.Loader {
	l := emailtemplates.NewLoader(nil)
	l.Register(key, emailtemplates.EmbeddedFallback{
		Subject: subject, HTMLBody: html, TextBody: text,
	})
	return l
}

func TestTemplateClient_Send_BuildsEnvelope(t *testing.T) {
	sender := &captureSender{}
	loader := loaderWith("dunning_day_5",
		"Payment failed for {{.store_name}}",
		"<p>Hi {{.store_name}}, day {{.day}}</p>",
		"Hi {{.store_name}}, day {{.day}}")

	c := email.NewTemplateClient(loader, sender, "noreply@mark8ly.com", slog.Default())

	err := c.Send(context.Background(), email.TemplateDunningDay5, "merchant@example.com", map[string]any{
		"store_name": "Acme",
		"day":        5,
		"tenant_id":  "tenant-123",
	})
	if err != nil {
		t.Fatalf("Send returned %v, want nil", err)
	}

	msg := sender.last(t)
	if msg.To != "merchant@example.com" {
		t.Errorf("To = %q", msg.To)
	}
	if msg.From != "noreply@mark8ly.com" {
		t.Errorf("From = %q", msg.From)
	}
	if msg.FromName != "Mark8ly Billing" {
		t.Errorf("FromName = %q", msg.FromName)
	}
	if msg.Subject != "Payment failed for Acme" {
		t.Errorf("Subject = %q", msg.Subject)
	}
	if !strings.Contains(msg.HTMLBody, "day 5") {
		t.Errorf("HTMLBody = %q", msg.HTMLBody)
	}
	if !strings.Contains(msg.TextBody, "day 5") {
		t.Errorf("TextBody = %q", msg.TextBody)
	}
	if msg.CustomArgs["product"] != "mark8ly" {
		t.Errorf("product arg = %q", msg.CustomArgs["product"])
	}
	if msg.CustomArgs["kind"] != "dunning_day_5" {
		t.Errorf("kind arg = %q", msg.CustomArgs["kind"])
	}
	if msg.CustomArgs["tenant_id"] != "tenant-123" {
		t.Errorf("tenant_id arg = %q", msg.CustomArgs["tenant_id"])
	}
}

// The whole point of #381: never report success for mail we did not send.
func TestTemplateClient_Send_UndeliverableNeverReachesSender(t *testing.T) {
	sender := &captureSender{}
	loader := loaderWith("dunning_day_5", "s", "<p>h</p>", "t")
	c := email.NewTemplateClient(loader, sender, "noreply@mark8ly.com", slog.Default())

	for _, to := range []string{"", "b0a1-uuid-not-an-email", "billing+7f3a@mark8ly.local"} {
		err := c.Send(context.Background(), email.TemplateDunningDay5, to, map[string]any{})
		if err == nil {
			t.Fatalf("Send(%q) = nil, want ErrUndeliverable", to)
		}
		if !errors.Is(err, email.ErrUndeliverable) {
			t.Errorf("Send(%q) err = %v, want ErrUndeliverable", to, err)
		}
	}
	if len(sender.msgs) != 0 {
		t.Errorf("sender received %d messages, want 0", len(sender.msgs))
	}
}

func TestTemplateClient_Send_UnknownKeyIsRenderFailure(t *testing.T) {
	sender := &captureSender{}
	c := email.NewTemplateClient(emailtemplates.NewLoader(nil), sender, "noreply@mark8ly.com", slog.Default())

	err := c.Send(context.Background(), email.TemplateDunningDay5, "merchant@example.com", map[string]any{})
	if err == nil {
		t.Fatal("Send with unregistered key = nil, want error")
	}
	if !errors.Is(err, email.ErrRender) {
		t.Errorf("err = %v, want ErrRender", err)
	}
	if email.SkipReason(err) != email.ReasonRenderFailed {
		t.Errorf("SkipReason = %q, want %q", email.SkipReason(err), email.ReasonRenderFailed)
	}
}

func TestTemplateClient_Send_TransportFailurePropagates(t *testing.T) {
	sender := &captureSender{err: errors.New("sendgrid 503")}
	loader := loaderWith("dunning_day_5", "s", "<p>h</p>", "t")
	c := email.NewTemplateClient(loader, sender, "noreply@mark8ly.com", slog.Default())

	err := c.Send(context.Background(), email.TemplateDunningDay5, "merchant@example.com", map[string]any{})
	if err == nil {
		t.Fatal("Send = nil, want transport error")
	}
	if !errors.Is(err, email.ErrTransport) {
		t.Errorf("err = %v, want ErrTransport", err)
	}
	if email.SkipReason(err) != email.ReasonTransportFailed {
		t.Errorf("SkipReason = %q, want %q", email.SkipReason(err), email.ReasonTransportFailed)
	}
}

func TestSkipReason_UndeliverableWins(t *testing.T) {
	err := email.ValidateRecipient("x@y.local")
	if got := email.SkipReason(err); got != email.ReasonPlaceholderAddress {
		t.Errorf("SkipReason = %q, want %q", got, email.ReasonPlaceholderAddress)
	}
	if got := email.SkipReason(errors.New("boom")); got != email.ReasonUnknown {
		t.Errorf("SkipReason(unrelated) = %q, want %q", got, email.ReasonUnknown)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /tmp/m8-381/services/marketplace-api && go test ./internal/email/ -run TestTemplateClient -v
```

Expected: FAIL — `undefined: email.NewTemplateClient`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/email/template_client.go`:

```go
package email

// template_client.go — the production implementation of Client.
//
// Before this landed, Client had exactly one implementation (NoOpClient),
// so no merchant had ever received a dunning notice, trial reminder,
// payment-action reminder, win-back promo or trial-billed confirmation
// (#381). This adapter renders a template key through the shared
// emailtemplates registry — the same one orderdoc and giftcard use, so
// operators can reword billing copy without a deploy — and hands the
// finished envelope to the shared SendGrid→Resend Sender chain.
//
// Contract, and the reason the delivery counters can be trusted: Send
// returns nil if and only if a provider accepted the message. Every other
// outcome returns a classified error. Callers map that to a
// billing_emails_skipped_total{template,reason} increment; they must never
// increment a *_sent_total counter for it.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
)

// fromName is the display name on every billing email.
const fromName = "Mark8ly Billing"

// Sentinels so callers can classify a failure without string matching.
var (
	// ErrRender means the template could not be rendered — an unknown key,
	// or a published DB override with broken syntax.
	ErrRender = errors.New("template render failed")
	// ErrTransport means every configured provider refused the message.
	ErrTransport = errors.New("transport failed")
)

// Additional reason labels, complementing those in recipient.go.
const (
	ReasonRenderFailed    = "render_failed"
	ReasonTransportFailed = "transport_failed"
	ReasonUnknown         = "unknown"
)

// SkipReason maps a Send error onto a stable metric label.
func SkipReason(err error) string {
	if reason, ok := UndeliverableReason(err); ok {
		return reason
	}
	switch {
	case errors.Is(err, ErrRender):
		return ReasonRenderFailed
	case errors.Is(err, ErrTransport):
		return ReasonTransportFailed
	default:
		return ReasonUnknown
	}
}

type templateClient struct {
	loader *emailtemplates.Loader
	sender Sender
	from   string
	logger *slog.Logger
}

// NewTemplateClient returns the production Client. loader and sender are
// required; a nil logger falls back to slog.Default().
func NewTemplateClient(loader *emailtemplates.Loader, sender Sender, from string, logger *slog.Logger) Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &templateClient{loader: loader, sender: sender, from: from, logger: logger}
}

// Send renders `template` with `data` and delivers it to `to`.
//
// `to` is an email address. It used to be a store UUID at four call sites
// and a Stripe customer ID at a fifth; callers now resolve the address from
// store_subscriptions.email before calling.
func (c *templateClient) Send(ctx context.Context, template TemplateID, to string, data map[string]any) error {
	if err := ValidateRecipient(to); err != nil {
		c.logger.Warn("billing email: undeliverable recipient; not sending",
			"template", string(template), "reason", SkipReason(err))
		return err
	}

	rendered, err := c.loader.Render(ctx, string(template), data)
	if err != nil {
		c.logger.Error("billing email: render failed",
			"template", string(template), "err", err.Error())
		return fmt.Errorf("email: render %s: %w: %v", template, ErrRender, err)
	}

	msg := Message{
		From:     c.from,
		FromName: fromName,
		To:       strings.TrimSpace(to),
		Subject:  rendered.Subject,
		HTMLBody: rendered.HTMLBody,
		TextBody: rendered.TextBody,
		// Wave 1.5 attribution — the same shape the five working mailers
		// emit, so the notification-service webhook receiver groups these
		// without parsing subjects, and #348's send log picks them up free.
		CustomArgs: map[string]string{
			"product": "mark8ly",
			"kind":    string(template),
		},
	}
	if tenantID, ok := data["tenant_id"].(string); ok && tenantID != "" {
		msg.CustomArgs["tenant_id"] = tenantID
	}

	if err := c.sender.Send(ctx, msg); err != nil {
		c.logger.Error("billing email: transport failed",
			"template", string(template), "err", err.Error())
		return fmt.Errorf("email: send %s: %w: %v", template, ErrTransport, err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /tmp/m8-381/services/marketplace-api && go test ./internal/email/ -v
```

Expected: PASS, including the pre-existing `noop_test.go` and `fallback_test.go` cases.

- [ ] **Step 5: Commit**

```bash
cd /tmp/m8-381 && git add services/marketplace-api/internal/email/template_client.go services/marketplace-api/internal/email/template_client_test.go
git commit -m "feat(email): add the production template client over the shared sender chain"
```

---

### Task 4: The eleven templates and their registration

Registers every billing key against the shared `emailtemplates.Loader` so an
operator can reword copy without a deploy, with an embedded fallback that keeps
mail flowing when the DB row is absent (which it is for all of them today — see
the spec's "no seed rows" decision).

`hosted_invoice_url` is guarded with `{{if}}` because it is only populated by
`invoice.payment_action_required` and cleared on `invoice.paid`
(`subscription/models.go:150-155`), so dunning rows may legitimately lack it.

**Files:**
- Modify: `internal/email/client.go` (add `TemplateWinBack`)
- Modify: `internal/subscription/lifecycle/winback.go:20-23` (drop the local const, use `email.TemplateWinBack`)
- Replace: `internal/email/templates.go` (currently a 3-line stub)
- Create: `internal/email/templates_content.go`
- Test: `internal/email/templates_test.go`

**Interfaces:**
- Consumes: `emailtemplates.Loader.Register`, `emailtemplates.EmbeddedFallback`, `TemplateID` constants.
- Produces:
  - `func RegisterFallbacks(loader *emailtemplates.Loader)`
  - `func BillingTemplateKeys() []TemplateID`
  - `const TemplateWinBack TemplateID = "win_back_day30"`

- [ ] **Step 1: Write the failing test**

Create `internal/email/templates_test.go`:

```go
package email_test

import (
	"context"
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
)

// sampleVars covers every field any billing template references, so one
// map renders all eleven. Extra keys are harmless.
func sampleVars() map[string]any {
	return map[string]any{
		"store_id":           "11111111-2222-3333-4444-555555555555",
		"tenant_id":          "66666666-7777-8888-9999-000000000000",
		"store_name":         "Acme Supply Co",
		"day":                5,
		"offset":             "t_minus_7",
		"days_remaining":     7,
		"has_payment_method": false,
		"plan":               "growth",
		"period":             "monthly",
		"promo":              "20%-off-6-months",
		"hosted_invoice_url": "https://invoice.stripe.com/i/test",
	}
}

func TestRegisterFallbacks_EveryKeyRenders(t *testing.T) {
	loader := emailtemplates.NewLoader(nil)
	email.RegisterFallbacks(loader)

	keys := email.BillingTemplateKeys()
	if len(keys) != 11 {
		t.Fatalf("BillingTemplateKeys() has %d keys, want 11", len(keys))
	}

	for _, key := range keys {
		t.Run(string(key), func(t *testing.T) {
			r, err := loader.Render(context.Background(), string(key), sampleVars())
			if err != nil {
				t.Fatalf("Render(%s) failed: %v", key, err)
			}
			if strings.TrimSpace(r.Subject) == "" {
				t.Error("empty subject")
			}
			if strings.TrimSpace(r.TextBody) == "" {
				t.Error("empty text body")
			}
			if !strings.Contains(r.HTMLBody, "<!doctype html>") {
				t.Error("html body is not a full document — chrome not applied")
			}
			if strings.Contains(r.HTMLBody, "<!--BODY-->") {
				t.Error("chrome placeholder was not substituted")
			}
			// Every template addresses the merchant by store name.
			if !strings.Contains(r.HTMLBody, "Acme Supply Co") {
				t.Error("html body does not reference store_name")
			}
			// No unresolved template actions should survive rendering.
			if strings.Contains(r.HTMLBody, "{{") || strings.Contains(r.TextBody, "{{") {
				t.Error("unrendered template action left in output")
			}
		})
	}
}

func TestRegisterFallbacks_HostedInvoiceURLIsOptional(t *testing.T) {
	loader := emailtemplates.NewLoader(nil)
	email.RegisterFallbacks(loader)

	vars := sampleVars()
	vars["hosted_invoice_url"] = ""

	r, err := loader.Render(context.Background(), string(email.TemplateDunningDay5), vars)
	if err != nil {
		t.Fatalf("Render without invoice url failed: %v", err)
	}
	if strings.Contains(r.HTMLBody, "href=\"\"") {
		t.Error("emitted an empty href instead of omitting the CTA")
	}
}

func TestBillingTemplateKeys_IncludesWinBack(t *testing.T) {
	var found bool
	for _, k := range email.BillingTemplateKeys() {
		if k == email.TemplateWinBack {
			found = true
		}
	}
	if !found {
		t.Error("win_back_day30 missing from the billing catalog")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /tmp/m8-381/services/marketplace-api && go test ./internal/email/ -run 'TestRegisterFallbacks|TestBillingTemplateKeys' -v
```

Expected: FAIL — `undefined: email.RegisterFallbacks`, `undefined: email.BillingTemplateKeys`, `undefined: email.TemplateWinBack`.

- [ ] **Step 3a: Move the win-back template constant into the email package**

In `internal/email/client.go`, add to the existing `const (...)` block, after `TemplateTrialStartedBilled`:

```go
	// Day-30 post-expiry win-back promo. Lived in the lifecycle package
	// until #381; moved here so the billing catalog is complete in one
	// place and the email package can register its fallback without
	// importing lifecycle (which imports email).
	TemplateWinBack TemplateID = "win_back_day30"
```

In `internal/subscription/lifecycle/winback.go`, delete lines 20-23 (the
`TemplateWinBack` doc comment and const) and change the reference at what is
currently line 76 from `TemplateWinBack` to `email.TemplateWinBack`.

- [ ] **Step 3b: Write the template content**

Create `internal/email/templates_content.go`:

```go
package email

// templates_content.go — embedded fallback copy for the eleven billing
// templates (#381).
//
// These are FALLBACKS. The authoritative copy, once an operator writes one,
// is the published row in email_templates; the loader prefers it and only
// reaches for these when the row is absent, draft, or the DB is unreachable.
// Keep them plain and provider-agnostic: no external images, no web fonts,
// table-based layout, inline styles only.
//
// Brand: paper (#F7F6F2) page, white card, ink (#0E0E0C) text, moss
// (#2D4A2B) for the single call to action. One accent, no decoration.

// chromeHTML wraps every fragment. <!--BODY--> is substituted at
// registration time; it is deliberately an HTML comment rather than a
// template action so the chrome itself is never parsed as a template.
const chromeHTML = `<!doctype html>
<html>
<body style="margin:0;padding:0;background:#F7F6F2;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#F7F6F2;">
<tr><td align="center" style="padding:40px 16px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;background:#FFFFFF;border-radius:6px;">
<tr><td style="padding:40px;font-family:'Source Sans 3',-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;font-size:16px;line-height:1.6;color:#0E0E0C;">
<!--BODY-->
<hr style="border:none;border-top:1px solid #E4E2DC;margin:32px 0 16px;">
<p style="font-size:12px;line-height:1.5;color:#6B6A64;margin:0;">Mark8ly · You are receiving this because you run a store on Mark8ly.</p>
</td></tr></table>
</td></tr></table>
</body>
</html>`

// h1 is the shared editorial headline style — Source Serif 4, per the
// brand direction that the serif carries the brand.
const h1 = `style="font-family:'Source Serif 4',Georgia,serif;font-size:26px;line-height:1.25;font-weight:600;margin:0 0 20px;"`

// cta renders the single moss button. Guarded by {{if}} at each call site
// so a missing URL omits the button rather than emitting an empty href.
const ctaOpen = `<p style="margin:28px 0 0;"><a href="`
const ctaMid = `" style="display:inline-block;background:#2D4A2B;color:#FFFFFF;text-decoration:none;padding:12px 22px;border-radius:6px;font-weight:600;">`
const ctaClose = `</a></p>`

// --- subjects -------------------------------------------------------

var billingSubjects = map[TemplateID]string{
	TemplateDunningDay5:           `We could not process your payment for {{.store_name}}`,
	TemplateDunningDay7:           `Action needed to keep {{.store_name}} open`,
	TemplatePaymentActionReminder: `Confirm your payment for {{.store_name}}`,
	TemplateTrialNoPMT15:          `15 days left in your {{.store_name}} trial`,
	TemplateTrialNoPMT10:          `10 days left in your {{.store_name}} trial`,
	TemplateTrialNoPMT7:           `One week left in your {{.store_name}} trial`,
	TemplateTrialNoPMT3:           `3 days left in your {{.store_name}} trial`,
	TemplateTrialNoPMT1:           `Your {{.store_name}} trial ends tomorrow`,
	TemplateTrialHasPMT1:          `Your {{.plan}} plan for {{.store_name}} starts tomorrow`,
	TemplateTrialStartedBilled:    `Your {{.plan}} plan for {{.store_name}} is active`,
	TemplateWinBack:               `Come back to Mark8ly — 20% off six months`,
}

// --- HTML fragments -------------------------------------------------

var billingHTMLFragments = map[TemplateID]string{
	TemplateDunningDay5: `<h1 ` + h1 + `>Your payment did not go through</h1>
<p>We tried to charge the card on file for <strong>{{.store_name}}</strong> and it was declined. Your store is still open.</p>
<p>Updating your payment method now avoids any interruption.</p>
{{if .hosted_invoice_url}}` + ctaOpen + `{{.hosted_invoice_url}}` + ctaMid + `Complete your payment` + ctaClose + `{{else}}<p>Open your Mark8ly billing settings to update your card.</p>{{end}}`,

	TemplateDunningDay7: `<h1 ` + h1 + `>Action needed to keep {{.store_name}} open</h1>
<p>It has been {{.day}} days since your payment failed. If it stays unpaid, <strong>{{.store_name}}</strong> will be suspended and your storefront will stop serving customers.</p>
<p>This is reversible right up until suspension.</p>
{{if .hosted_invoice_url}}` + ctaOpen + `{{.hosted_invoice_url}}` + ctaMid + `Pay now` + ctaClose + `{{else}}<p>Open your Mark8ly billing settings to update your card.</p>{{end}}`,

	TemplatePaymentActionReminder: `<h1 ` + h1 + `>One step left to confirm your payment</h1>
<p>Your bank asked for extra confirmation before charging the card for <strong>{{.store_name}}</strong>. Until you approve it, the payment is not complete.</p>
{{if .hosted_invoice_url}}` + ctaOpen + `{{.hosted_invoice_url}}` + ctaMid + `Confirm payment` + ctaClose + `{{else}}<p>Open your Mark8ly billing settings to finish confirming.</p>{{end}}`,

	TemplateTrialNoPMT15: `<h1 ` + h1 + `>15 days left in your trial</h1>
<p><strong>{{.store_name}}</strong> has {{.days_remaining}} days left on the free trial. There is no card on file yet.</p>
<p>Adding one now means your storefront keeps serving customers the moment the trial ends. Nothing is charged until then.</p>`,

	TemplateTrialNoPMT10: `<h1 ` + h1 + `>10 days left in your trial</h1>
<p><strong>{{.store_name}}</strong> has {{.days_remaining}} days left on the free trial, and no payment method on file.</p>
<p>Choose a plan whenever you are ready — you will not be charged before the trial ends.</p>`,

	TemplateTrialNoPMT7: `<h1 ` + h1 + `>One week left in your trial</h1>
<p><strong>{{.store_name}}</strong> has {{.days_remaining}} days of trial remaining. There is still no card on file.</p>
<p>Without one, your storefront stops serving customers when the trial ends.</p>`,

	TemplateTrialNoPMT3: `<h1 ` + h1 + `>3 days left in your trial</h1>
<p><strong>{{.store_name}}</strong> has {{.days_remaining}} days left. Adding a payment method takes a minute and keeps everything running.</p>`,

	TemplateTrialNoPMT1: `<h1 ` + h1 + `>Your trial ends tomorrow</h1>
<p>Tomorrow the free trial for <strong>{{.store_name}}</strong> ends. With no payment method on file, the storefront will stop serving customers.</p>
<p>Your products, orders and settings are kept — adding a card restores the store immediately.</p>`,

	TemplateTrialHasPMT1: `<h1 ` + h1 + `>Your {{.plan}} plan starts tomorrow</h1>
<p>The free trial for <strong>{{.store_name}}</strong> ends tomorrow, and your <strong>{{.plan}}</strong> plan begins. We will charge the card on file — nothing for you to do.</p>
<p>If you would rather change plan, you can do that before the charge.</p>`,

	TemplateTrialStartedBilled: `<h1 ` + h1 + `>Your {{.plan}} plan is active</h1>
<p>Thank you — the first payment for <strong>{{.store_name}}</strong> went through and your <strong>{{.plan}}</strong> plan is now active, billed {{.period}}.</p>
<p>Your receipt is in your billing settings.</p>`,

	TemplateWinBack: `<h1 ` + h1 + `>Your store is still here</h1>
<p><strong>{{.store_name}}</strong> has been closed for a month. Everything — products, orders, settings — is exactly as you left it.</p>
<p>If you want to pick it back up, we will take <strong>20% off your first six months</strong>.</p>`,
}

// --- text fragments -------------------------------------------------

var billingTextFragments = map[TemplateID]string{
	TemplateDunningDay5: `Your payment did not go through

We tried to charge the card on file for {{.store_name}} and it was declined. Your store is still open.

Updating your payment method now avoids any interruption.
{{if .hosted_invoice_url}}
Complete your payment: {{.hosted_invoice_url}}
{{else}}
Open your Mark8ly billing settings to update your card.
{{end}}
Mark8ly`,

	TemplateDunningDay7: `Action needed to keep {{.store_name}} open

It has been {{.day}} days since your payment failed. If it stays unpaid, {{.store_name}} will be suspended and your storefront will stop serving customers.

This is reversible right up until suspension.
{{if .hosted_invoice_url}}
Pay now: {{.hosted_invoice_url}}
{{else}}
Open your Mark8ly billing settings to update your card.
{{end}}
Mark8ly`,

	TemplatePaymentActionReminder: `One step left to confirm your payment

Your bank asked for extra confirmation before charging the card for {{.store_name}}. Until you approve it, the payment is not complete.
{{if .hosted_invoice_url}}
Confirm payment: {{.hosted_invoice_url}}
{{else}}
Open your Mark8ly billing settings to finish confirming.
{{end}}
Mark8ly`,

	TemplateTrialNoPMT15: `15 days left in your trial

{{.store_name}} has {{.days_remaining}} days left on the free trial. There is no card on file yet.

Adding one now means your storefront keeps serving customers the moment the trial ends. Nothing is charged until then.

Mark8ly`,

	TemplateTrialNoPMT10: `10 days left in your trial

{{.store_name}} has {{.days_remaining}} days left on the free trial, and no payment method on file.

Choose a plan whenever you are ready — you will not be charged before the trial ends.

Mark8ly`,

	TemplateTrialNoPMT7: `One week left in your trial

{{.store_name}} has {{.days_remaining}} days of trial remaining. There is still no card on file.

Without one, your storefront stops serving customers when the trial ends.

Mark8ly`,

	TemplateTrialNoPMT3: `3 days left in your trial

{{.store_name}} has {{.days_remaining}} days left. Adding a payment method takes a minute and keeps everything running.

Mark8ly`,

	TemplateTrialNoPMT1: `Your trial ends tomorrow

Tomorrow the free trial for {{.store_name}} ends. With no payment method on file, the storefront will stop serving customers.

Your products, orders and settings are kept — adding a card restores the store immediately.

Mark8ly`,

	TemplateTrialHasPMT1: `Your {{.plan}} plan starts tomorrow

The free trial for {{.store_name}} ends tomorrow, and your {{.plan}} plan begins. We will charge the card on file — nothing for you to do.

If you would rather change plan, you can do that before the charge.

Mark8ly`,

	TemplateTrialStartedBilled: `Your {{.plan}} plan is active

Thank you — the first payment for {{.store_name}} went through and your {{.plan}} plan is now active, billed {{.period}}.

Your receipt is in your billing settings.

Mark8ly`,

	TemplateWinBack: `Your store is still here

{{.store_name}} has been closed for a month. Everything — products, orders, settings — is exactly as you left it.

If you want to pick it back up, we will take 20% off your first six months.

Mark8ly`,
}
```

- [ ] **Step 3c: Write the registration**

Replace the whole of `internal/email/templates.go` with:

```go
// Package-level template registration for billing mail (#381).
//
// Every key here is registered against the shared emailtemplates.Loader at
// boot, exactly as orderdoc and giftcard do. Registration makes a key
// overridable from the operator console; it does not seed a DB row, so
// until someone authors one these embedded fallbacks are what sends.
package email

import (
	"fmt"
	"strings"

	"github.com/mark8ly/marketplace-api/internal/emailtemplates"
)

// bodyMarker is the substitution point in chromeHTML.
const bodyMarker = "<!--BODY-->"

// billingTemplateKeys is the catalog, in the order an operator would
// encounter them across a subscription's life.
var billingTemplateKeys = []TemplateID{
	TemplateTrialNoPMT15,
	TemplateTrialNoPMT10,
	TemplateTrialNoPMT7,
	TemplateTrialNoPMT3,
	TemplateTrialNoPMT1,
	TemplateTrialHasPMT1,
	TemplateTrialStartedBilled,
	TemplatePaymentActionReminder,
	TemplateDunningDay5,
	TemplateDunningDay7,
	TemplateWinBack,
}

// BillingTemplateKeys returns a copy of the catalog. A copy, so a caller
// cannot reorder or truncate the registry it is reading.
func BillingTemplateKeys() []TemplateID {
	out := make([]TemplateID, len(billingTemplateKeys))
	copy(out, billingTemplateKeys)
	return out
}

// RegisterFallbacks binds every billing template's embedded fallback to
// the loader. Call once at boot, before any cron can fire.
//
// Panics if a key is missing content — a programming error that would
// otherwise surface as a silently unsent email at 09:05 UTC.
func RegisterFallbacks(loader *emailtemplates.Loader) {
	for _, key := range billingTemplateKeys {
		subject, ok := billingSubjects[key]
		if !ok {
			panic(fmt.Sprintf("email: no subject registered for template %q", key))
		}
		htmlFragment, ok := billingHTMLFragments[key]
		if !ok {
			panic(fmt.Sprintf("email: no html fragment registered for template %q", key))
		}
		textBody, ok := billingTextFragments[key]
		if !ok {
			panic(fmt.Sprintf("email: no text fragment registered for template %q", key))
		}
		loader.Register(string(key), emailtemplates.EmbeddedFallback{
			Subject:  subject,
			HTMLBody: strings.Replace(chromeHTML, bodyMarker, htmlFragment, 1),
			TextBody: textBody,
		})
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /tmp/m8-381/services/marketplace-api && go build ./... && go test ./internal/email/ ./internal/subscription/lifecycle/ -v
```

Expected: PASS. All eleven subtests of `TestRegisterFallbacks_EveryKeyRenders` render, and `lifecycle` still compiles against `email.TemplateWinBack`.

- [ ] **Step 5: Commit**

```bash
cd /tmp/m8-381 && git add services/marketplace-api/internal/email/ services/marketplace-api/internal/subscription/lifecycle/winback.go
git commit -m "feat(email): register embedded fallbacks for the eleven billing templates"
```

---

### Task 5: The `store_subscriptions.email` column

**Files:**
- Create: `migrations/000104_store_subscriptions_email.up.sql`
- Create: `migrations/000104_store_subscriptions_email.down.sql`
- Modify: `internal/subscription/models.go` (add a field after `StripeCustomerID` at :110)

**Interfaces:**
- Produces: `subscription.StoreSubscription.Email *string` mapped to column `email`.

- [ ] **Step 1: Write the migration**

Create `migrations/000104_store_subscriptions_email.up.sql`:

```sql
-- Billing mail needs somewhere to send to (#381). Until now dunning, trial
-- reminder, payment-action, win-back and trial-billed mailers passed a store
-- UUID as the "to" address, which was harmless only because every one of them
-- was wired to a no-op client.
--
-- NULL means "not known yet" and is explicitly expected: customer.updated only
-- fires on change, so rows predating this column stay NULL until
-- cmd/backfill-email reads them from Stripe. A NULL recipient is refused by
-- email.ValidateRecipient and counted as skipped — never sent, never counted
-- as delivered.
--
-- citext because email comparison is case-insensitive and we do not want two
-- rows differing only by case to read as different merchants.
CREATE EXTENSION IF NOT EXISTS citext;

ALTER TABLE store_subscriptions ADD COLUMN IF NOT EXISTS email CITEXT;
```

Create `migrations/000104_store_subscriptions_email.down.sql`:

```sql
-- DESTRUCTIVE: drops every merchant billing address mirrored from Stripe.
-- Recoverable by re-running cmd/backfill-email, since Stripe remains the
-- source of truth — but every cron reverts to sending nothing in the
-- meantime, because a NULL recipient is refused.
--
-- The citext extension is deliberately NOT dropped: other objects may depend
-- on it, and dropping a shared extension during a rollback is how you take
-- down unrelated tables.
ALTER TABLE store_subscriptions DROP COLUMN IF EXISTS email;
```

- [ ] **Step 2: Add the model field**

In `internal/subscription/models.go`, immediately after the `StripeCustomerID` line (:110):

```go
	// Email is the merchant's billing address, mirrored from the Stripe
	// customer by handleCustomerUpdated and backfilled by cmd/backfill-email
	// (migration 104, #381). NULL means "not known yet" — every mailer
	// refuses to send and counts a skip rather than guessing.
	Email *string `gorm:"column:email;type:citext"`
```

- [ ] **Step 3: Apply and verify the migration**

```bash
cd /tmp/m8-381/services/marketplace-api && make migrate-up 2>/dev/null || \
  go run ./cmd/migrate -database "$TEST_DATABASE_URL" up
docker run --rm postgres:15 psql "postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable" \
  -c "\d store_subscriptions" | grep -i email
```

Expected: a row showing `email | citext |`.

If the connection is refused, `docker start dev-postgres-1` and retry. If no
`migrate` target exists, check `Makefile` for the project's migrate invocation
and use that — do not invent a new migration runner.

- [ ] **Step 4: Verify the model compiles and nothing regressed**

```bash
cd /tmp/m8-381/services/marketplace-api && go build ./... && go vet ./internal/subscription/...
```

Expected: clean.

- [ ] **Step 5: Commit**

```bash
cd /tmp/m8-381 && git add services/marketplace-api/migrations/000104_store_subscriptions_email.up.sql services/marketplace-api/migrations/000104_store_subscriptions_email.down.sql services/marketplace-api/internal/subscription/models.go
git commit -m "feat(subscription): add store_subscriptions.email for billing mail"
```

---

### Task 6: Mirror the address from `customer.updated`

The webhook that already updates this table for `has_default_payment_method`
gains one field and one column in the same statement — no new subscription, no
new query, no new failure mode.

**Files:**
- Modify: `internal/billing/dispatch/handlers.go:413-444` (`handleCustomerUpdated`)
- Test: `internal/billing/dispatch/handlers_customer_updated_test.go`

**Interfaces:**
- Consumes: `subscription.StoreSubscription.Email` (Task 5).
- Produces: nothing new; behaviour change only.

- [ ] **Step 1: Write the failing test**

Create `internal/billing/dispatch/handlers_customer_updated_test.go`. This is an
integration test because it asserts a real `UPDATE`:

The package's integration tests open the database with
`pkg/testdb.NewDB(t, tables...)`, which truncates the named tables before and
after each test — see `dispatcher_test.go:21`. Use that; do not write a new
helper.

```go
//go:build integration

package dispatch_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing/dispatch"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestHandleCustomerUpdated_MirrorsEmail(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")
	ctx := context.Background()

	customerID := "cus_" + uuid.NewString()[:12]
	sub := &subscription.StoreSubscription{
		TenantID:         uuid.New(),
		StoreID:          uuid.New(),
		StripeCustomerID: customerID,
	}
	require.NoError(t, db.Create(sub).Error)

	raw := []byte(`{"data":{"object":{"id":"` + customerID + `","email":"merchant@example.com","invoice_settings":{"default_payment_method":"pm_123"}}}}`)

	if err := dispatch.HandleCustomerUpdatedForTest(ctx, db, raw); err != nil {
		t.Fatalf("handleCustomerUpdated: %v", err)
	}

	var got subscription.StoreSubscription
	if err := db.Where("stripe_customer_id = ?", customerID).First(&got).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Email == nil || *got.Email != "merchant@example.com" {
		t.Errorf("Email = %v, want merchant@example.com", got.Email)
	}
	if !got.HasDefaultPaymentMethod {
		t.Error("HasDefaultPaymentMethod regressed to false")
	}
}

// An event without an email must not blank an address we already have.
func TestHandleCustomerUpdated_AbsentEmailPreservesExisting(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions")
	ctx := context.Background()

	existing := "keep@example.com"
	customerID := "cus_" + uuid.NewString()[:12]
	sub := &subscription.StoreSubscription{
		TenantID:         uuid.New(),
		StoreID:          uuid.New(),
		StripeCustomerID: customerID,
		Email:            &existing,
	}
	if err := db.Create(sub).Error; err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	raw := []byte(`{"data":{"object":{"id":"` + customerID + `","invoice_settings":{"default_payment_method":null}}}}`)

	if err := dispatch.HandleCustomerUpdatedForTest(ctx, db, raw); err != nil {
		t.Fatalf("handleCustomerUpdated: %v", err)
	}

	var got subscription.StoreSubscription
	if err := db.Where("stripe_customer_id = ?", customerID).First(&got).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Email == nil || *got.Email != existing {
		t.Errorf("Email = %v, want it preserved as %q", got.Email, existing)
	}
}
```

`handleCustomerUpdated` is unexported and the tests live in `dispatch_test`, so
add an export-for-test shim in a new `//go:build integration` file inside the
package:

```go
//go:build integration

package dispatch

import (
	"context"

	"gorm.io/gorm"
)

// HandleCustomerUpdatedForTest exposes the unexported handler to the
// package's external integration tests.
func HandleCustomerUpdatedForTest(ctx context.Context, tx *gorm.DB, raw []byte) error {
	return handleCustomerUpdated(ctx, tx, raw)
}
```

Note `testdb.NewDB` **skips** rather than fails when the database is
unreachable. A green run with no output is not proof; check for `--- PASS` on
the named tests, not merely `exit=0`.

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /tmp/m8-381/services/marketplace-api && ps aux | grep -c "[g]o test -tags=integration"
TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable" \
  go test -tags=integration -p 1 ./internal/billing/dispatch/ -run TestHandleCustomerUpdated -v
```

Expected: the `grep -c` prints `0` (nothing else is running a suite), then FAIL —
`Email = <nil>, want merchant@example.com`.

- [ ] **Step 3: Write minimal implementation**

In `internal/billing/dispatch/handlers.go`, extend the anonymous payload struct
in `handleCustomerUpdated` to parse the address, and widen the `UPDATE`:

```go
	var e struct {
		Data struct {
			Object struct {
				Customer        string `json:"id"`
				Email           string `json:"email"`
				InvoiceSettings struct {
					DefaultPaymentMethod *string `json:"default_payment_method"`
				} `json:"invoice_settings"`
			} `json:"object"`
		} `json:"data"`
	}
```

Replace the `UPDATE` with:

```go
	// email is written only when the event carries one. Stripe omits the
	// field on some replays, and an absent field must not blank an address
	// we already hold — COALESCE on the parameter, not on the column, so an
	// empty string is treated as "no value in this event".
	email := strings.TrimSpace(e.Data.Object.Email)

	res := tx.WithContext(ctx).Exec(`
		UPDATE store_subscriptions
		SET has_default_payment_method = ?,
		    email                      = COALESCE(NULLIF(?, ''), email),
		    updated_at                 = now()
		WHERE stripe_customer_id = ?`,
		hasPM, email, customer,
	)
	if res.Error != nil {
		return fmt.Errorf("dispatch: customer.updated has_default_payment_method: %w", res.Error)
	}
	return nil
```

Add `"strings"` to the file's imports if it is not already there.

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /tmp/m8-381/services/marketplace-api && \
TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable" \
  go test -tags=integration -p 1 ./internal/billing/dispatch/ -run TestHandleCustomerUpdated -v
```

Expected: PASS, both cases.

- [ ] **Step 5: Commit**

```bash
cd /tmp/m8-381 && git add services/marketplace-api/internal/billing/dispatch/
git commit -m "feat(billing): mirror the Stripe customer email onto store_subscriptions"
```

---

### Task 7: Backfill addresses for rows predating the column

`customer.updated` only fires on change, so every existing subscription stays
NULL without this. Modeled on `cmd/backfill-has-pm`, which exists for exactly
this shape.

**Files:**
- Modify: `internal/billing/stripe/customer.go` (add `GetCustomerEmail`)
- Create: `cmd/backfill-email/main.go`
- Test: `internal/billing/stripe/customer_email_test.go`

**Interfaces:**
- Consumes: `billingstripe.Client`, `subscription.StoreSubscription.Email` (Task 5).
- Produces: `func GetCustomerEmail(ctx context.Context, c *Client, customerID string) (string, error)`

- [ ] **Step 1: Write the failing test**

Create `internal/billing/stripe/customer_email_test.go`:

```go
package stripe_test

import (
	"context"
	"testing"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

func TestGetCustomerEmail_EmptyCustomerID(t *testing.T) {
	got, err := billingstripe.GetCustomerEmail(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("err = %v, want nil for an empty customer id", err)
	}
	if got != "" {
		t.Errorf("email = %q, want empty", got)
	}
}
```

Check the package's existing `customer_test.go` for an established fake-transport
harness. If one exists, add a case asserting a customer payload with
`"email":"merchant@example.com"` yields that address; if not, this
guard-clause test plus the backfill's integration coverage is sufficient — do
not stand up a new HTTP fake for one getter.

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /tmp/m8-381/services/marketplace-api && go test ./internal/billing/stripe/ -run TestGetCustomerEmail -v
```

Expected: FAIL — `undefined: stripe.GetCustomerEmail`.

- [ ] **Step 3: Write minimal implementation**

Append to `internal/billing/stripe/customer.go`:

```go
// GetCustomerEmail returns the billing address Stripe holds for a customer,
// or "" when the customer has none. Used by cmd/backfill-email to populate
// store_subscriptions.email for rows predating migration 104 (#381).
//
// An empty customerID is not an error — it means the subscription was never
// bootstrapped against Stripe, which the caller skips.
func GetCustomerEmail(ctx context.Context, c *Client, customerID string) (string, error) {
	if customerID == "" {
		return "", nil
	}
	params := &sdk.CustomerRetrieveParams{}
	params.Context = ctx
	cu, err := c.sdk.V1Customers.Retrieve(ctx, customerID, params)
	if err != nil {
		return "", toAPIError(err)
	}
	if cu == nil {
		return "", nil
	}
	return cu.Email, nil
}
```

- [ ] **Step 4: Write the backfill command**

Create `cmd/backfill-email/main.go`:

```go
// Command backfill-email populates store_subscriptions.email for rows that
// pre-date migration 104. customer.updated webhooks only fire on change, so
// historical customers with an email already set in Stripe won't have the
// column populated without this script (#381).
//
// Run once per environment after migration 104 lands. Idempotent — safe to
// re-run; it always re-reads from Stripe and writes the current value.
//
// Addresses Stripe reports that we would refuse to send to (the
// billing+<store_id>@mark8ly.local placeholders minted by subscription
// bootstrap) are counted as `Placeholder` and NOT written: storing one would
// only move the refusal from send time to a column nobody reads.
//
// Multi-tenant safety: each row is keyed by stripe_customer_id (1:1 with
// (tenant_id, store_id)). A failure on one row does not block other rows or
// other tenants — failures are logged and the script continues.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"gorm.io/gorm"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/db"
)

func main() {
	var (
		batchSize int
		throttle  time.Duration
		dryRun    bool
	)
	flag.IntVar(&batchSize, "batch", 200, "rows fetched per DB scan")
	flag.DurationVar(&throttle, "throttle", 50*time.Millisecond, "sleep between Stripe API calls (rate-limit hedge)")
	flag.BoolVar(&dryRun, "dry-run", false, "log changes without writing")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Error("backfill-email: DATABASE_URL not set")
		os.Exit(1)
	}
	stripeKey := os.Getenv("STRIPE_BILLING_SECRET_KEY")
	if stripeKey == "" {
		log.Error("backfill-email: STRIPE_BILLING_SECRET_KEY not set")
		os.Exit(1)
	}

	conn, err := db.Open(databaseURL)
	if err != nil {
		log.Error("backfill-email: db open failed", "err", err)
		os.Exit(1)
	}
	stripeClient := billingstripe.New(stripeKey)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	stats, err := run(ctx, conn, stripeClient, batchSize, throttle, dryRun, log)
	if err != nil {
		log.Error("backfill-email: run failed", "err", err, "stats", stats)
		os.Exit(1)
	}
	log.Info("backfill-email: done", "stats", stats)
}

type runStats struct {
	Scanned     int
	Updated     int
	Unchanged   int
	NoneInStripe int
	Placeholder int // Stripe holds an address we would refuse to send to
	Failed      int
}

func run(ctx context.Context, conn *gorm.DB, sc *billingstripe.Client, batchSize int, throttle time.Duration, dryRun bool, log *slog.Logger) (runStats, error) {
	var stats runStats

	// Keyset pagination via id ordering — avoids OFFSET drift on a live table
	// and keeps memory bounded regardless of subscription count.
	lastID := ""

	for {
		var rows []subscription.StoreSubscription
		q := conn.WithContext(ctx).
			Where("stripe_customer_id <> ''").
			Order("id ASC").
			Limit(batchSize)
		if lastID != "" {
			q = q.Where("id > ?", lastID)
		}
		if err := q.Find(&rows).Error; err != nil {
			return stats, err
		}
		if len(rows) == 0 {
			return stats, nil
		}

		for i := range rows {
			row := &rows[i]
			stats.Scanned++
			lastID = row.ID.String()

			addr, err := billingstripe.GetCustomerEmail(ctx, sc, row.StripeCustomerID)
			if err != nil {
				stats.Failed++
				log.Warn("backfill-email: stripe lookup failed; skipping",
					"tenant_id", row.TenantID.String(),
					"store_id", row.StoreID.String(),
					"stripe_customer_id", row.StripeCustomerID,
					"err", err.Error())
				time.Sleep(throttle)
				continue
			}

			if addr == "" {
				stats.NoneInStripe++
				time.Sleep(throttle)
				continue
			}

			if err := email.ValidateRecipient(addr); err != nil {
				stats.Placeholder++
				log.Warn("backfill-email: stripe holds an undeliverable address; not storing",
					"tenant_id", row.TenantID.String(),
					"store_id", row.StoreID.String(),
					"reason", email.SkipReason(err))
				time.Sleep(throttle)
				continue
			}

			if row.Email != nil && *row.Email == addr {
				stats.Unchanged++
				time.Sleep(throttle)
				continue
			}

			if dryRun {
				log.Info("backfill-email: dry-run would update",
					"tenant_id", row.TenantID.String(),
					"store_id", row.StoreID.String())
				stats.Updated++
				time.Sleep(throttle)
				continue
			}

			// Per-row UPDATE keyed by id — narrowest possible blast radius.
			res := conn.WithContext(ctx).Exec(`
				UPDATE store_subscriptions
				SET email = ?, updated_at = now()
				WHERE id = ?`,
				addr, row.ID,
			)
			if res.Error != nil {
				stats.Failed++
				log.Warn("backfill-email: update failed",
					"store_id", row.StoreID.String(), "err", res.Error.Error())
				time.Sleep(throttle)
				continue
			}
			stats.Updated++
			time.Sleep(throttle)
		}
	}
}
```

Confirm `billingstripe.New` and `db.Open` are the exact constructor names used
by `cmd/backfill-has-pm/main.go`; mirror whatever it does rather than guessing.

- [ ] **Step 5: Run tests and build**

```bash
cd /tmp/m8-381/services/marketplace-api && go build ./... && go test ./internal/billing/stripe/ -run TestGetCustomerEmail -v && go run ./cmd/backfill-email -h
```

Expected: build clean, test PASS, and `-h` prints the three flags.

- [ ] **Step 6: Commit**

```bash
cd /tmp/m8-381 && git add services/marketplace-api/internal/billing/stripe/ services/marketplace-api/cmd/backfill-email/
git commit -m "feat(billing): backfill store_subscriptions.email from Stripe"
```

---

### Task 8: Claim-first idempotency for the unguarded crons

Trial reminders and payment-action reminders already claim a slot before
sending. Dunning and win-back re-derive eligibility on every run, so a second
run the same day re-sends. Behind a no-op that was harmless; with a real
transport it is duplicate billing mail. One generic table rather than two more
bespoke ones.

**Files:**
- Create: `migrations/000105_billing_email_sends.up.sql`
- Create: `migrations/000105_billing_email_sends.down.sql`
- Create: `internal/subscription/email_claim.go`
- Test: `internal/subscription/email_claim_integration_test.go`

**Interfaces:**
- Produces: `func ClaimEmailSend(ctx context.Context, db *gorm.DB, subscriptionID uuid.UUID, templateKey, periodKey string, now time.Time) (bool, error)` — returns `true` when this caller won the claim and must send, `false` when someone already has it.

- [ ] **Step 1: Write the migration**

Create `migrations/000105_billing_email_sends.up.sql`:

```sql
-- Claim-first idempotency for billing mail (#381).
--
-- trial_reminders and payment_action_reminders already do this per-feature.
-- Dunning and win-back did not: both re-derive eligibility on every run
-- (dunning from audit_logs date arithmetic, win-back from an updated_at
-- window), so a second run on the same day re-sent to the same merchants.
-- That was invisible while every send was a no-op.
--
-- Generic rather than two more bespoke tables: four near-identical marker
-- tables is three too many, and this is where trial_reminders and
-- payment_action_reminders should eventually be folded in.
--
-- period_key disambiguates repeats of the same template for the same
-- subscription: the target date for dunning, the window-start date for
-- win-back. template_key alone would suppress a legitimate day-7 notice
-- after a day-5 one; period_key alone would collide across templates.
CREATE TABLE IF NOT EXISTS billing_email_sends (
    subscription_id UUID        NOT NULL,
    template_key    TEXT        NOT NULL,
    period_key      TEXT        NOT NULL,
    sent_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (subscription_id, template_key, period_key)
);

-- Supports the operational question "what did we send this store, when?"
-- without scanning; the primary key already covers the claim path.
CREATE INDEX IF NOT EXISTS billing_email_sends_sent_at_idx
    ON billing_email_sends (sent_at DESC);
```

Create `migrations/000105_billing_email_sends.down.sql`:

```sql
-- DESTRUCTIVE: drops every claim marker. Rolling back past 105 means the
-- next dunning or win-back run re-sends to every merchant currently inside
-- an eligibility window — real duplicate mail to real merchants, not just
-- lost bookkeeping.
DROP INDEX IF EXISTS billing_email_sends_sent_at_idx;
DROP TABLE IF EXISTS billing_email_sends;
```

- [ ] **Step 2: Write the failing test**

Create `internal/subscription/email_claim_integration_test.go`:

```go
//go:build integration

package subscription_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestClaimEmailSend_SecondClaimLoses(t *testing.T) {
	db := testdb.NewDB(t, "billing_email_sends")
	ctx := context.Background()
	subID := uuid.New()
	now := time.Now().UTC()

	won, err := subscription.ClaimEmailSend(ctx, db, subID, "dunning_day_5", "2026-08-26", now)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if !won {
		t.Fatal("first claim did not win")
	}

	won, err = subscription.ClaimEmailSend(ctx, db, subID, "dunning_day_5", "2026-08-26", now)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if won {
		t.Error("second claim won — duplicate mail would be sent")
	}
}

func TestClaimEmailSend_DifferentTemplateAndPeriodBothWin(t *testing.T) {
	db := testdb.NewDB(t, "billing_email_sends")
	ctx := context.Background()
	subID := uuid.New()
	now := time.Now().UTC()

	if won, err := subscription.ClaimEmailSend(ctx, db, subID, "dunning_day_5", "2026-08-26", now); err != nil || !won {
		t.Fatalf("day_5 claim: won=%v err=%v", won, err)
	}
	// A day-7 notice after a day-5 one is legitimate, not a duplicate.
	if won, err := subscription.ClaimEmailSend(ctx, db, subID, "dunning_day_7", "2026-08-26", now); err != nil || !won {
		t.Errorf("day_7 same date should win: won=%v err=%v", won, err)
	}
	// The same template in a later period is also legitimate.
	if won, err := subscription.ClaimEmailSend(ctx, db, subID, "dunning_day_5", "2026-09-26", now); err != nil || !won {
		t.Errorf("day_5 later period should win: won=%v err=%v", won, err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
cd /tmp/m8-381/services/marketplace-api && ps aux | grep -c "[g]o test -tags=integration"
TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable" \
  go test -tags=integration -p 1 ./internal/subscription/ -run TestClaimEmailSend -v
```

Expected: `0` running suites, then FAIL — `undefined: subscription.ClaimEmailSend`.

- [ ] **Step 4: Write minimal implementation**

Create `internal/subscription/email_claim.go`:

```go
package subscription

// email_claim.go — claim-first idempotency for billing mail (#381).
//
// The contract mirrors payment_action_reminders: claim the slot BEFORE
// sending, and never release it on a send failure. That makes delivery
// at-most-once — a transient provider error costs the merchant that one
// notice rather than risking a duplicate. The failure is visible through
// the caller's skipped counter and Warn log.

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ClaimEmailSend attempts to claim (subscriptionID, templateKey, periodKey).
//
// Returns true when this caller inserted the row and is therefore the one
// that must send. Returns false when the slot was already claimed — by
// another pod, or by an earlier run of the same cron today.
func ClaimEmailSend(ctx context.Context, db *gorm.DB, subscriptionID uuid.UUID, templateKey, periodKey string, now time.Time) (bool, error) {
	res := db.WithContext(ctx).Exec(`
		INSERT INTO billing_email_sends (subscription_id, template_key, period_key, sent_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT DO NOTHING`,
		subscriptionID, templateKey, periodKey, now,
	)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}
```

- [ ] **Step 5: Apply the migration and run the test**

```bash
cd /tmp/m8-381/services/marketplace-api && go run ./cmd/migrate -database "$TEST_DATABASE_URL" up 2>/dev/null || make migrate-up
TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable" \
  go test -tags=integration -p 1 ./internal/subscription/ -run TestClaimEmailSend -v
```

Expected: PASS, both tests.

- [ ] **Step 6: Commit**

```bash
cd /tmp/m8-381 && git add services/marketplace-api/migrations/000105_billing_email_sends.up.sql services/marketplace-api/migrations/000105_billing_email_sends.down.sql services/marketplace-api/internal/subscription/email_claim.go services/marketplace-api/internal/subscription/email_claim_integration_test.go
git commit -m "feat(subscription): add claim-first billing_email_sends idempotency table"
```

---

### Task 9: Dunning cron — real recipient, claim, honest counters

**Files:**
- Modify: `internal/subscription/dunning/dunning_emails.go` (`emailRow` :38-42, `runForDay` :91-131, constructor :59)
- Test: `internal/subscription/dunning/dunning_emails_integration_test.go` (extend — it exists) and `testhelpers_integration_test.go` (add one seeder)

**Interfaces:**
- Consumes: `subscription.ClaimEmailSend` (Task 8), `email.SkipReason` (Task 3), `dunning.SkipCounter` (Task 2), `StoreSubscription.Email` (Task 5).
- Produces: `func (s *SendDunningEmails) WithSkipCounter(c SkipCounter) *SendDunningEmails`

A builder method rather than a new constructor parameter, so existing call
sites and tests keep compiling.

- [ ] **Step 1: Write the failing test**

The dunning cron is only reachable through the database, and this package
already has the seams: `testdb.NewDB`, plus `seedStore` in
`testhelpers_integration_test.go:27`. Follow
`dunning_emails_integration_test.go` for how a `past_due` row and its matching
`audit_logs` transition are seeded — read it before writing this, and reuse its
helpers rather than adding parallel ones.

Add to `internal/subscription/dunning/dunning_emails_integration_test.go`:

```go
// stubClient records recipients and can fail on demand.
type stubClient struct {
	sent []string
	err  error
}

func (c *stubClient) Send(_ context.Context, _ email.TemplateID, to string, _ map[string]any) error {
	if c.err != nil {
		return c.err
	}
	c.sent = append(c.sent, to)
	return nil
}

// stubVec / stubSkip count increments by label.
type stubVec struct{ n map[string]int }

func (s *stubVec) WithDay(day string) dunning.CounterIncrementer {
	if s.n == nil {
		s.n = map[string]int{}
	}
	return stubInc{s.n, day}
}

type stubSkip struct{ n map[string]int }

func (s *stubSkip) WithTemplateReason(template, reason string) dunning.CounterIncrementer {
	if s.n == nil {
		s.n = map[string]int{}
	}
	return stubInc{s.n, template + "/" + reason}
}

type stubInc struct {
	n   map[string]int
	key string
}

func (s stubInc) Inc() { s.n[s.key]++ }

func TestDunning_UndeliverableCountsSkippedNotSent(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores", "audit_logs", "billing_email_sends")
	now := time.Now().UTC()

	placeholder := "billing+7f3a@mark8ly.local"
	seedPastDueSubscription(t, db, now.AddDate(0, 0, -5), &placeholder)

	client := &stubClient{}
	sent, skipped := &stubVec{}, &stubSkip{}
	cron := dunning.NewSendDunningEmails(db, client, nil, sent, func() time.Time { return now }).
		WithSkipCounter(skipped)

	require.NoError(t, cron.Run(context.Background()))

	require.Empty(t, client.sent, "mailed a .local address")
	require.Zero(t, sent.n["day_5"], "sent counter incremented for mail never sent — the #381 lie")
	require.Equal(t, 1, skipped.n["dunning_day_5/placeholder_address"])
}

func TestDunning_SecondRunSameDayDoesNotResend(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores", "audit_logs", "billing_email_sends")
	now := time.Now().UTC()

	addr := "merchant@example.com"
	seedPastDueSubscription(t, db, now.AddDate(0, 0, -5), &addr)

	client := &stubClient{}
	newCron := func() *dunning.SendDunningEmails {
		return dunning.NewSendDunningEmails(db, client, nil, &stubVec{}, func() time.Time { return now })
	}

	require.NoError(t, newCron().Run(context.Background()))
	require.Len(t, client.sent, 1, "first run should send exactly once")

	require.NoError(t, newCron().Run(context.Background()))
	require.Len(t, client.sent, 1, "second run re-sent — duplicate dunning mail")
}
```

Write `seedPastDueSubscription(t, db, transitionedAt, email)` in
`testhelpers_integration_test.go` alongside `seedStore`: it inserts a
`store_subscriptions` row with `status='past_due'` and the given `email`, calls
`seedStore` for the name, and inserts the `audit_logs` row with
`action='subscription.state_transition'`, `metadata->>'to_status'='past_due'`
and `created_at=transitionedAt`. Copy the exact column set from the existing
`dunning_emails_integration_test.go` seeding rather than inferring it.

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /tmp/m8-381/services/marketplace-api && go test ./internal/subscription/dunning/ -run TestDunning_Undeliverable -v
```

Expected: FAIL — `WithSkipCounter` undefined, or the sent counter incrementing.

- [ ] **Step 3: Write minimal implementation**

Widen `emailRow` in `internal/subscription/dunning/dunning_emails.go`:

```go
// emailRow is the minimal projection returned by the dunning email query.
// Email and StoreName come from the merchant-facing side of the join: the
// address to send to, and the name the templates address them by.
type emailRow struct {
	SubscriptionID   uuid.UUID
	StoreID          string
	TenantID         string
	Email            *string
	StoreName        string
	HostedInvoiceURL *string
}
```

Add the skip counter field and builder:

```go
// WithSkipCounter attaches the counter for emails deliberately not sent.
// Optional: nil means skips are logged but not counted.
func (s *SendDunningEmails) WithSkipCounter(c SkipCounter) *SendDunningEmails {
	s.skip = c
	return s
}
```

...with `skip SkipCounter` added to the `SendDunningEmails` struct.

Widen the query in `runForDay` to select the new columns:

```go
	err := s.db.WithContext(ctx).Raw(`
		SELECT DISTINCT
		    ss.id   AS subscription_id,
		    ss.store_id,
		    ss.tenant_id,
		    ss.email,
		    ss.hosted_invoice_url,
		    COALESCE(st.name, 'your store') AS store_name
		FROM store_subscriptions ss
		JOIN audit_logs a ON a.store_id = ss.store_id
		LEFT JOIN stores st ON st.id = ss.store_id
		WHERE ss.status = ?
		  AND a.action = 'subscription.state_transition'
		  AND a.metadata->>'to_status' = ?
		  AND date_trunc('day', a.created_at) = date_trunc('day', ? ::timestamptz)`,
		string(subscription.StatusPastDue),
		string(subscription.StatusPastDue),
		targetDay,
	).Scan(&rows).Error
```

`LEFT JOIN` with a `COALESCE` default, because `stores` is a lazily-populated
local projection (`internal/stores/models.go:1-9`) — a missing row must not
drop a merchant from the dunning ladder.

Replace the send loop body:

```go
	dayLabel := fmt.Sprintf("day_%d", t.Day)
	periodKey := targetDay.Format("2006-01-02")

	for _, r := range rows {
		// Claim before sending. Dunning re-derives eligibility from
		// audit_logs on every run, so without this a second run on the
		// same day re-sends to the same merchants (#381).
		won, err := subscription.ClaimEmailSend(ctx, s.db, r.SubscriptionID, string(t.Template), periodKey, now)
		if err != nil {
			s.logger.Error("dunning email: claim failed; skipping row",
				"day", t.Day, "store_id", r.StoreID, "err", err.Error())
			continue
		}
		if !won {
			continue // already claimed by another pod or an earlier run
		}

		to := ""
		if r.Email != nil {
			to = *r.Email
		}
		invoiceURL := ""
		if r.HostedInvoiceURL != nil {
			invoiceURL = *r.HostedInvoiceURL
		}

		if err := s.emailCl.Send(ctx, t.Template, to, map[string]any{
			"store_id":           r.StoreID,
			"tenant_id":          r.TenantID,
			"store_name":         r.StoreName,
			"day":                t.Day,
			"hosted_invoice_url": invoiceURL,
		}); err != nil {
			// Never increment the sent counter here. Before #381 this
			// branch was unreachable because the client was a no-op that
			// always returned nil, so the counter reported deliveries
			// that never happened.
			s.logger.Warn("dunning email not sent",
				"day", t.Day, "store_id", r.StoreID,
				"reason", email.SkipReason(err), "err", err.Error())
			if s.skip != nil {
				s.skip.WithTemplateReason(string(t.Template), email.SkipReason(err)).Inc()
			}
			continue
		}
		if s.counter != nil {
			s.counter.WithDay(dayLabel).Inc()
		}
	}
	return nil
```

`runForDay` needs `now` — it currently derives `targetDay` from a `now`
parameter; pass the same value through to `ClaimEmailSend`. Add `"github.com/google/uuid"` to the imports.

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /tmp/m8-381/services/marketplace-api && go build ./... && go test ./internal/subscription/dunning/ -v
```

Expected: PASS, including the pre-existing dunning tests.

- [ ] **Step 5: Commit**

```bash
cd /tmp/m8-381 && git add services/marketplace-api/internal/subscription/dunning/dunning_emails.go services/marketplace-api/internal/subscription/dunning/dunning_emails_test.go
git commit -m "fix(dunning): send to a real address, claim before sending, stop overcounting"
```

---

### Task 10: Win-back cron — real recipient, claim, and a corrected comment

`winback.go:25-27` currently claims *"It is idempotent by design: stores already
past 31 days are never selected, so double-runs on the same day produce the same
send."* Producing the same send twice is not idempotence — it is a duplicate.
This is the "documentation promising more than the code enforces" pattern the
handoff calls out across three branches, so the comment is corrected in the same
change that makes it true.

**Files:**
- Modify: `internal/subscription/lifecycle/winback.go` (struct :32-37, comment :25-31, `sendOne` :72-89)
- Create: `internal/subscription/lifecycle/winback_integration_test.go` — **the `lifecycle` package has no test files whatsoever today.** This is the first one. There is no local helper to reuse; use `pkg/testdb.NewDB` as every other integration suite does.

**Interfaces:**
- Consumes: `subscription.ClaimEmailSend` (Task 8), `email.SkipReason` and `email.TemplateWinBack` (Tasks 3, 4).
- Produces:
  - `type CounterIncrementer interface{ Inc() }`
  - `type SkipCounter interface{ WithTemplateReason(template, reason string) CounterIncrementer }`
  - `func (c *WinBackCron) WithSkipCounter(sc SkipCounter) *WinBackCron`

Interfaces are declared here rather than imported from `dunning`, because
`lifecycle` does not depend on `dunning` today and adding that edge to satisfy
two three-line interfaces is the wrong trade. Consumer-side interface
declaration is idiomatic Go.

- [ ] **Step 1: Write the failing test**

Create `internal/subscription/lifecycle/winback_integration_test.go`:

```go
//go:build integration

package lifecycle_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/email"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/internal/subscription/lifecycle"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

type stubClient struct {
	sent []string
	err  error
}

func (c *stubClient) Send(_ context.Context, _ email.TemplateID, to string, _ map[string]any) error {
	if c.err != nil {
		return c.err
	}
	c.sent = append(c.sent, to)
	return nil
}

type stubSkip struct{ n map[string]int }

func (s *stubSkip) WithTemplateReason(template, reason string) lifecycle.CounterIncrementer {
	if s.n == nil {
		s.n = map[string]int{}
	}
	return stubInc{s.n, template + "/" + reason}
}

type stubInc struct {
	n   map[string]int
	key string
}

func (s stubInc) Inc() { s.n[s.key]++ }

// seedExpired inserts an expired subscription whose updated_at sits inside
// the 30-31 day win-back window, then forces updated_at past GORM's
// autoupdate (which would otherwise stamp now()).
func seedExpired(t *testing.T, db *gorm.DB, now time.Time, addr *string) subscription.StoreSubscription {
	t.Helper()
	sub := subscription.StoreSubscription{
		TenantID:         uuid.New(),
		StoreID:          uuid.New(),
		StripeCustomerID: "cus_" + uuid.NewString()[:12],
		Status:           subscription.StatusExpired,
		Email:            addr,
	}
	require.NoError(t, db.Create(&sub).Error)
	require.NoError(t, db.Exec(
		`UPDATE store_subscriptions SET updated_at = ? WHERE id = ?`,
		now.Add(-30*24*time.Hour-time.Hour), sub.ID).Error)
	return sub
}

func TestWinBack_UndeliverableIsSkippedNotSent(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "billing_email_sends")
	now := time.Now().UTC()

	placeholder := "billing+7f3a@mark8ly.local"
	seedExpired(t, db, now, &placeholder)

	client := &stubClient{}
	skipped := &stubSkip{}
	cron := lifecycle.NewWinBackCron(db, client, nil, func() time.Time { return now }).
		WithSkipCounter(skipped)

	require.NoError(t, cron.Run(context.Background()))

	require.Empty(t, client.sent, "mailed a .local address")
	require.Equal(t, 1, skipped.n["win_back_day30/placeholder_address"])
}

func TestWinBack_SecondRunSameDayDoesNotResend(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "billing_email_sends")
	now := time.Now().UTC()

	addr := "merchant@example.com"
	seedExpired(t, db, now, &addr)

	client := &stubClient{}
	newCron := func() *lifecycle.WinBackCron {
		return lifecycle.NewWinBackCron(db, client, nil, func() time.Time { return now })
	}

	require.NoError(t, newCron().Run(context.Background()))
	require.Len(t, client.sent, 1, "first run should send exactly once")

	require.NoError(t, newCron().Run(context.Background()))
	require.Len(t, client.sent, 1, "second run re-sent — duplicate win-back mail")
}
```

Add `"gorm.io/gorm"` to the imports for `seedExpired`. If `Create` does not
honour an explicit `updated_at`, the follow-up `UPDATE` above is what places
the row inside the window — keep it even if it looks redundant.

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /tmp/m8-381/services/marketplace-api && go test ./internal/subscription/lifecycle/ -run TestWinBack -v
```

Expected: FAIL — the second run sends again, and `WithSkipCounter` is undefined.

- [ ] **Step 3: Write minimal implementation**

Replace the misleading doc comment on `WinBackCron` (:25-31) with:

```go
// WinBackCron sends a 20%-off-6-months promo email to expired stores at day 30
// post-expiry (§15.3).
//
// Idempotence comes from the billing_email_sends claim, NOT from the window
// query. The window selects the same rows on every run within the same day —
// before #381 the comment here claimed that was idempotent, which it was only
// because the client was a no-op that never sent anything.
//
// NOTE: The actual promo code attachment (P10 promo service) is deferred.
type WinBackCron struct {
	db     *gorm.DB
	mailer email.Client
	logger *slog.Logger
	clock  func() time.Time
	skip   SkipCounter
}
```

Add the interfaces and builder:

```go
// CounterIncrementer is a one-method counter so tests can stub it.
type CounterIncrementer interface{ Inc() }

// SkipCounter counts win-back emails deliberately not sent, labeled by
// template and reason. Declared here rather than imported from the dunning
// package so lifecycle keeps its current dependency set.
type SkipCounter interface {
	WithTemplateReason(template, reason string) CounterIncrementer
}

// WithSkipCounter attaches the skipped-delivery counter. Optional.
func (c *WinBackCron) WithSkipCounter(sc SkipCounter) *WinBackCron {
	c.skip = sc
	return c
}
```

`Run` must pass the window start to `sendOne` so it can form a period key.
Change the loop at :66-68 to:

```go
	periodKey := windowStart.Format("2006-01-02")
	for i := range rows {
		c.sendOne(ctx, &rows[i], periodKey, now)
	}
```

Replace `sendOne` entirely:

```go
func (c *WinBackCron) sendOne(ctx context.Context, row *subscription.StoreSubscription, periodKey string, now time.Time) {
	won, err := subscription.ClaimEmailSend(ctx, c.db, row.ID, string(email.TemplateWinBack), periodKey, now)
	if err != nil {
		c.logger.Error("lifecycle: win-back claim failed; skipping",
			"store_id", row.StoreID, "err", err.Error())
		return
	}
	if !won {
		return // already sent for this window
	}

	to := ""
	if row.Email != nil {
		to = *row.Email
	}

	err = c.mailer.Send(ctx, email.TemplateWinBack, to, map[string]any{
		"store_id":   row.StoreID.String(),
		"tenant_id":  row.TenantID.String(),
		"store_name": subscription.StoreNameFor(ctx, c.db, row.StoreID),
		"promo":      "20%-off-6-months",
	})
	if err != nil {
		c.logger.Warn("lifecycle: win-back email not sent",
			"store_id", row.StoreID, "tenant_id", row.TenantID,
			"reason", email.SkipReason(err), "err", err.Error())
		if c.skip != nil {
			c.skip.WithTemplateReason(string(email.TemplateWinBack), email.SkipReason(err)).Inc()
		}
		return
	}
	c.logger.Info("lifecycle: win-back email sent",
		"store_id", row.StoreID, "tenant_id", row.TenantID)
}
```

The win-back query selects full `StoreSubscription` rows, so `Email` is already
loaded — no query change needed. `StoreNameFor` is a single scalar lookup per
row, acceptable because this cron processes a handful of rows a day; the
dunning ladder joins instead because it is the higher-volume path.

**This task creates `StoreNameFor`**, because it is its first consumer. Create
`internal/subscription/store_name.go` — the `subscription` package, not
`lifecycle` or `dunning`, because Task 11 needs it too and neither of those
packages is importable from the other:

```go
package subscription

// store_name.go — the merchant-facing store name for email copy.
//
// The crons load StoreSubscription rows, which carry no name; the name lives
// in the local `stores` projection. A scalar lookup per row is acceptable on
// the reminder paths, which process tens of rows daily. The dunning ladder
// joins instead, being the higher-volume path.

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// StoreNameFor returns the store's display name, or "your store" when the
// local projection has no row yet. Never returns an error: a cosmetic field
// must not be able to stop a billing email.
func StoreNameFor(ctx context.Context, db *gorm.DB, storeID uuid.UUID) string {
	var name string
	err := db.WithContext(ctx).
		Raw(`SELECT name FROM stores WHERE id = ?`, storeID).
		Scan(&name).Error
	if err != nil || name == "" {
		return "your store"
	}
	return name
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /tmp/m8-381/services/marketplace-api && go build ./... && go test ./internal/subscription/lifecycle/ -v
TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable" \
  go test -tags=integration -p 1 ./internal/subscription/lifecycle/ -run TestWinBack -v
```

Expected: PASS both, and the second run sends zero.

- [ ] **Step 5: Commit**

```bash
cd /tmp/m8-381 && git add services/marketplace-api/internal/subscription/lifecycle/
git commit -m "fix(lifecycle): claim win-back sends and correct the idempotence claim"
```

---

### Task 11: Trial reminders and payment-action reminders

Both already claim before sending, so they need the recipient and the skipped
counter only — no new marker.

**Files:**
- Modify: `internal/subscription/dunning/trial_reminders.go` (`processOne` :144-184, struct :64-70)
- Modify: `internal/subscription/dunning/payment_action_reminders.go` (`processOne` :113-141, struct :36-41)
- Test: extend the two packages' existing test files

**Interfaces:**
- Consumes: `email.SkipReason`, `dunning.SkipCounter`, `StoreSubscription.Email`.
- Produces:
  - `func (s *SendTrialReminders) WithSkipCounter(c SkipCounter) *SendTrialReminders`
  - `func (s *SendPaymentActionReminders) WithSkipCounter(c SkipCounter) *SendPaymentActionReminders`

- [ ] **Step 1: Write the failing test**

Add to the trial-reminders test file:

```go
func TestTrialReminders_UndeliverableCountsSkippedNotSent(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores", "trial_reminders")
	now := time.Now().UTC()

	placeholder := "billing+7f3a@mark8ly.local"
	// A trial ending in 7 days, with no payment method — the t_minus_7 nudge.
	seedTrialSub(t, db, now.AddDate(0, 0, -83), nil, false, &placeholder)

	client := &stubClient{}
	sent, skipped := &stubVec{}, &stubSkip{}
	cron := dunning.NewSendTrialReminders(db, client, nil, sent, func() time.Time { return now }).
		WithSkipCounter(skipped)

	require.NoError(t, cron.Run(context.Background()))

	require.Empty(t, client.sent, "mailed a .local address")
	require.Zero(t, sent.n["t_minus_7"], "sent counter incremented for mail never sent")
	require.Equal(t, 1, skipped.n["trial_no_pm_t7/placeholder_address"])
}

// The claim is deliberately NOT released on failure: at-most-once beats a
// duplicate. This pins that contract so nobody "fixes" it into at-least-once.
func TestTrialReminders_FailedSendDoesNotReleaseTheClaim(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores", "trial_reminders")
	now := time.Now().UTC()

	addr := "merchant@example.com"
	seedTrialSub(t, db, now.AddDate(0, 0, -83), nil, false, &addr)

	client := &stubClient{err: errors.New("sendgrid 503")}
	cron := dunning.NewSendTrialReminders(db, client, nil, &stubVec{}, func() time.Time { return now })
	require.NoError(t, cron.Run(context.Background()))

	var claims int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM trial_reminders`).Scan(&claims).Error)
	require.EqualValues(t, 1, claims, "the burned slot must stay claimed")
}
```

Add the equivalent `TestPaymentActionReminders_UndeliverableCountsSkippedNotSent`
to `payment_action_reminders_integration_test.go`, expecting the label
`payment_action_reminder/placeholder_address`.

Both are integration tests. The seams already exist: `testdb.NewDB` for the
connection, `seedTrialSub` in `trial_reminders_extension_integration_test.go:29`
for a trialing subscription, and `seedStore` in
`testhelpers_integration_test.go:27` for the store name. `seedTrialSub` will
need an `email` argument — add it there rather than writing a second seeder, and
update its existing callers in the same commit. Reuse the `stubClient` /
`stubSkip` types added to this package in Task 9; do not declare them twice.

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /tmp/m8-381/services/marketplace-api && go test ./internal/subscription/dunning/ -run 'TestTrialReminders|TestPaymentActionReminders' -v
```

Expected: FAIL — `WithSkipCounter` undefined on both types.

- [ ] **Step 3: Write minimal implementation**

Add `skip SkipCounter` to both structs and a builder on each:

```go
// WithSkipCounter attaches the counter for emails deliberately not sent.
func (s *SendTrialReminders) WithSkipCounter(c SkipCounter) *SendTrialReminders {
	s.skip = c
	return s
}
```

```go
// WithSkipCounter attaches the counter for emails deliberately not sent.
func (s *SendPaymentActionReminders) WithSkipCounter(c SkipCounter) *SendPaymentActionReminders {
	s.skip = c
	return s
}
```

In `trial_reminders.go`, replace the `Send` call and its TODO comment block
(:160-179) with:

```go
	to := ""
	if row.Email != nil {
		to = *row.Email
	}

	if err := s.emailCl.Send(ctx, t.Template, to, map[string]any{
		"store_id":           row.StoreID.String(),
		"tenant_id":          row.TenantID.String(),
		"store_name":         subscription.StoreNameFor(ctx, s.db, row.StoreID),
		"offset":             t.OffsetKey,
		"days_remaining":     t.DaysBefore,
		"has_payment_method": t.HasPM,
		"plan":               string(row.Plan),
	}); err != nil {
		s.logger.Warn("trial reminder not sent",
			"store_id", row.StoreID.String(),
			"offset", t.OffsetKey,
			"reason", email.SkipReason(err),
			"err", err.Error())
		if s.skip != nil {
			s.skip.WithTemplateReason(string(t.Template), email.SkipReason(err)).Inc()
		}
		// Do not delete the idempotency row — that would risk a double-send.
		// At-most-once is the deliberate contract: see the spec, §6.
		return nil
	}
```

In `payment_action_reminders.go`, replace the `Send` call (:129-138) with:

```go
	to := ""
	if row.Email != nil {
		to = *row.Email
	}
	invoiceURL := ""
	if row.HostedInvoiceURL != nil {
		invoiceURL = *row.HostedInvoiceURL
	}

	if err := s.emailCl.Send(ctx, email.TemplatePaymentActionReminder, to, map[string]any{
		"store_id":           row.StoreID.String(),
		"tenant_id":          row.TenantID.String(),
		"store_name":         subscription.StoreNameFor(ctx, s.db, row.StoreID),
		"offset":             t.OffsetKey,
		"hosted_invoice_url": invoiceURL,
	}); err != nil {
		s.logger.Warn("SCA reminder not sent",
			"store_id", row.StoreID.String(), "offset", t.OffsetKey,
			"reason", email.SkipReason(err), "err", err.Error())
		if s.skip != nil {
			s.skip.WithTemplateReason(string(email.TemplatePaymentActionReminder), email.SkipReason(err)).Inc()
		}
		// Don't delete the idempotency row — we'd risk double-send on retry.
		return nil
	}
```

`subscription.StoreNameFor` already exists — **Task 10 created it**. Do not
create it again; just call it.

Add `"github.com/mark8ly/marketplace-api/internal/email"` to
`trial_reminders.go`'s imports if it is not already present.

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /tmp/m8-381/services/marketplace-api && go build ./... && go test ./internal/subscription/dunning/ -v
```

Expected: PASS across the whole package.

- [ ] **Step 5: Commit**

```bash
cd /tmp/m8-381 && git add services/marketplace-api/internal/subscription/dunning/
git commit -m "fix(dunning): send trial and SCA reminders to real addresses"
```

---

### Task 12: Trial-billed confirmation

No claim marker: this fires from the `invoice.paid` webhook, which
`handlers/webhooks/stripe.go:98-111` already deduplicates with an `InsertIfNew`
on `event_id` before dispatch ever runs.

**Files:**
- Modify: `internal/billing/dispatch/handlers.go:281-296`
- Test: `internal/billing/dispatch/dispatcher_test.go` (extend — there is no `handlers_test.go`)

**Interfaces:**
- Consumes: `StoreSubscription.Email` (Task 5), `email.SkipReason` (Task 3).
- Produces: nothing new.

- [ ] **Step 1: Write the failing test**

Add to `internal/billing/dispatch/dispatcher_test.go`, which is already
`//go:build integration` / `package dispatch_test` and uses
`testdb.NewDB(t, "store_subscriptions", "stripe_webhook_events")`. Follow
`TestDispatch_InvoicePaid_StampsFirstChargeAt_ClearsHostedURL` (:200) for how an
`invoice.paid` first charge is driven, and reuse its construction rather than
inventing a new harness:

```go
func TestInvoicePaid_TrialBilledUsesRealAddress(t *testing.T) {
	// sub is the StoreSubscription the handler loads; give it an address.
	addr := "merchant@example.com"
	client := &captureEmailClient{}

	runInvoicePaidFirstCharge(t, client, &addr)

	if len(client.recipients) != 1 {
		t.Fatalf("sent %d emails, want 1", len(client.recipients))
	}
	if client.recipients[0] != addr {
		t.Errorf("recipient = %q, want %q — a store UUID would bounce", client.recipients[0], addr)
	}
}

func TestInvoicePaid_TrialBilledWithoutAddressDoesNotFailTheWebhook(t *testing.T) {
	client := &captureEmailClient{}
	err := runInvoicePaidFirstCharge(t, client, nil)
	if err != nil {
		t.Errorf("webhook returned %v; email failure must stay non-fatal", err)
	}
}
```

`captureEmailClient` is a two-field stub implementing `email.Client` that
appends each `to` to a slice; attach it with the existing
`dispatch.New(...).WithEmail(client)` builder (`dispatcher.go:83-89`).
`runInvoicePaidFirstCharge` seeds a `store_subscriptions` row with
`first_charge_at` NULL and the given `email`, then dispatches an `invoice.paid`
payload — lift both from the `:200` test rather than writing them fresh.

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /tmp/m8-381/services/marketplace-api && go test ./internal/billing/dispatch/ -run TestInvoicePaid_TrialBilled -v
```

Expected: FAIL — recipient is the store UUID, not the address.

- [ ] **Step 3: Write minimal implementation**

Replace the block at `internal/billing/dispatch/handlers.go:281-296`:

```go
	if wasFirstCharge && d.emailCl != nil {
		to := ""
		if sub.Email != nil {
			to = *sub.Email
		}
		if sendErr := d.emailCl.Send(ctx, email.TemplateTrialStartedBilled, to, map[string]any{
			"store_id":   sub.StoreID.String(),
			"tenant_id":  sub.TenantID.String(),
			"store_name": "your store",
			"plan":       string(sub.Plan),
			"period":     string(sub.SubscriptionPeriod),
		}); sendErr != nil {
			// Don't fail the webhook — Stripe would retry, double-firing every
			// other side effect. Email failure is a soft error: log and move on.
			// Idempotency is preserved by first_charge_at being non-nil after
			// this UPDATE, so a retried invoice.paid event won't re-emit.
			slog.Default().Warn("dispatch: trial-billed email not sent",
				"store_id", sub.StoreID.String(),
				"reason", email.SkipReason(sendErr),
				"err", sendErr.Error())
		}
	}
```

Two things about this replacement:

`Dispatcher` has **no** logger field (`dispatcher.go:42-47` is
`emitter`/`recorder`/`emailCl`/`handlers`) and the `dispatch` package imports
`log/slog` nowhere today. Rather than widen the struct and every constructor
for one warning, use `slog.Default()` and add `"log/slog"` to the file's
imports. The process configures the default logger at boot, so this lands in
the same JSON stream as everything else.

The line it replaces — `_ = fmt.Errorf("dispatch: trial-billed email
(non-fatal): %w", sendErr)` — was dead code: it constructed an error and
discarded it, so every trial-billed send failure was invisible. That is why
this task exists at all.

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /tmp/m8-381/services/marketplace-api && go build ./... && go test ./internal/billing/dispatch/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /tmp/m8-381 && git add services/marketplace-api/internal/billing/dispatch/
git commit -m "fix(billing): send the trial-billed confirmation to a real address"
```

---

### Task 13: Wire it up — replace all three `NoOpClient` sites

The switch-flip. Until this task, everything built above is inert.

**Files:**
- Modify: `cmd/marketplace-api/main.go` — `:277-279` (registration), `:550-554` (after the sender), `:1599`, `:1761-1764`, `:1777-1790`, `:1825-1830`, `:1879-1881`

**Interfaces:**
- Consumes: everything from Tasks 1-12.
- Produces: a running service that sends billing mail.

- [ ] **Step 1: Register the billing fallbacks**

After `giftcard.RegisterFallbacks(templateLoader)` at `:279`:

```go
	// #381 — billing mail (dunning, trial cadence, payment-action, win-back,
	// trial-billed). Registered here so an operator can reword any of it from
	// the console without a deploy.
	email.RegisterFallbacks(templateLoader)
```

- [ ] **Step 2: Construct the one real client**

Immediately after the `emailSender := email.NewFromConfig(...)` block ends at
`:554`:

```go
	// billingEmailClient is the production email.Client. Before #381 the only
	// implementation was NoOpClient, wired at three sites below, so no
	// merchant had ever received a dunning notice, trial reminder,
	// payment-action reminder, win-back promo or trial-billed confirmation.
	// One instance shared by all three, so failover and attribution are
	// identical wherever billing mail originates.
	billingEmailClient := email.NewTemplateClient(templateLoader, emailSender, cfg.EmailFrom, log)
```

This must come after `emailSender`, which is why it is not next to the
`templateLoader` at `:277`.

- [ ] **Step 3: Replace the three no-op wirings**

At `:1599`, replace:

```go
		dispatcherEmailClient := email.NoOpClient{Logger: log}
```

with:

```go
		dispatcherEmailClient := billingEmailClient
```

At `:1761-1764`, replace the comment and assignment:

```go
	// P6 dunning + SCA recovery crons. Emails route through the NoOpClient
	...
	dunningEmailClient := email.NoOpClient{Logger: log}
```

with:

```go
	// P6 dunning + SCA recovery crons. Emails route through the real
	// template client as of #381 — recipients come from
	// store_subscriptions.email, and an unknown or placeholder address is
	// counted as skipped rather than reported as delivered.
	dunningEmailClient := billingEmailClient
```

At `:1879`, replace:

```go
	winBackEmailClient := email.NoOpClient{Logger: log}
```

with:

```go
	winBackEmailClient := billingEmailClient
```

- [ ] **Step 4: Attach the skip counters**

Chain `WithSkipCounter` onto each cron constructor. At `:1777-1779`:

```go
	dunningEmailsCron := dunning.NewSendDunningEmails(conn, dunningEmailClient, log,
		dunning.WrapPrometheusCounterVec(metrics.DunningEmailsSentTotal),
		nil,
	).WithSkipCounter(dunning.WrapPrometheusSkipCounter(metrics.BillingEmailsSkippedTotal))
```

At `:1788-1790`:

```go
	scaRemindersCron := dunning.NewSendPaymentActionReminders(conn, dunningEmailClient, log,
		dunning.WrapPrometheusCounterVec(metrics.PaymentActionRemindersSentTotal),
		nil,
	).WithSkipCounter(dunning.WrapPrometheusSkipCounter(metrics.BillingEmailsSkippedTotal))
```

At `:1825-1827`:

```go
	trialRemindersCron := dunning.NewSendTrialReminders(conn, dunningEmailClient, log,
		dunning.WrapPrometheusCounterVec(metrics.TrialRemindersSentTotal),
		nil,
	).WithSkipCounter(dunning.WrapPrometheusSkipCounter(metrics.BillingEmailsSkippedTotal))
```

At `:1880`:

```go
	winBackCron := lifecycle.NewWinBackCron(conn, winBackEmailClient, log, nil).
		WithSkipCounter(lifecycleSkipCounter{metrics.BillingEmailsSkippedTotal})
```

`lifecycle.SkipCounter` is a distinct interface from `dunning.SkipCounter`, so
add a two-line adapter near the other wiring helpers in `main.go`:

```go
// lifecycleSkipCounter adapts the shared skipped-emails CounterVec to
// lifecycle.SkipCounter. Separate from dunning's adapter because the two
// packages declare their own consumer-side interfaces.
type lifecycleSkipCounter struct{ cv *prometheus.CounterVec }

func (l lifecycleSkipCounter) WithTemplateReason(template, reason string) lifecycle.CounterIncrementer {
	return l.cv.WithLabelValues(template, reason)
}
```

`prometheus.Counter` already has `Inc()`, so it satisfies
`lifecycle.CounterIncrementer` directly. Preserve the existing argument values
at each call site — copy them from the current code rather than trusting the
`nil` shown above, which stands for whatever clock argument is already there.

- [ ] **Step 5: Verify the whole build and the full unit suite**

```bash
cd /tmp/m8-381/services/marketplace-api && go build ./... && go vet ./... && go vet -tags=integration ./...
grep -rn "NoOpClient" cmd/marketplace-api/main.go
```

Expected: build and both vets clean; the `grep` returns **nothing** — every
production wiring is gone. `NoOpClient` itself stays in the package: it is used
by tests and is a legitimate dev double.

- [ ] **Step 6: Commit**

```bash
cd /tmp/m8-381 && git add services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(marketplace-api): wire the real billing email client at all three sites"
```

---

## Final verification

Run this after Task 13, before requesting review. It is not optional: the
handoff records 22 packages / 191 tests already failing at baseline, and the
only way to know you added none is to diff both directions.

- [ ] **Step 1: Confirm no other integration suite is running**

```bash
ps aux | grep "[g]o test -tags=integration"
```

Expected: no output. Two suites against one database corrupt each other and the
resulting diff still looks authoritative.

- [ ] **Step 2: Capture the branch result**

```bash
cd /tmp/m8-381/services/marketplace-api && \
TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable" \
  go test -tags=integration -p 1 ./... 2>&1 | tee /tmp/m8-381-branch.txt | tail -40
```

- [ ] **Step 3: Capture the baseline from a throwaway worktree**

```bash
\
  git worktree add /tmp/m8-381-base d52b5cd2 && cd /tmp/m8-381-base/services/marketplace-api && \
TEST_DATABASE_URL="postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable" \
  go test -tags=integration -p 1 ./... 2>&1 | tee /tmp/m8-381-base.txt | tail -40
```

- [ ] **Step 4: Diff both directions**

```bash
grep -E "^(--- FAIL|FAIL)" /tmp/m8-381-branch.txt | sort -u > /tmp/branch-fails.txt
grep -E "^(--- FAIL|FAIL)" /tmp/m8-381-base.txt   | sort -u > /tmp/base-fails.txt
echo "=== NEW failures introduced by this branch (must be empty) ==="
comm -23 /tmp/branch-fails.txt /tmp/base-fails.txt
echo "=== failures the branch FIXED (informational) ==="
comm -13 /tmp/branch-fails.txt /tmp/base-fails.txt
```

Expected: the first list is **empty**. Anything in it is yours.

Then confirm the suites actually ran. `pkg/testdb.NewDB` **skips** when the
database is unreachable, and `exit=0` does not distinguish PASS from SKIP:

```bash
grep -c "^--- SKIP" /tmp/m8-381-branch.txt
grep -E "^--- (PASS|SKIP)" /tmp/m8-381-branch.txt | grep -E "ClaimEmailSend|Dunning_|WinBack_|TrialReminders_|HandleCustomerUpdated"
```

Every test this plan added must appear as `--- PASS`. A `--- SKIP` there means
the run proved nothing.

- [ ] **Step 5: Clean up the baseline worktree**

```bash
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly && git worktree remove /tmp/m8-381-base --force
```

- [ ] **Step 6: Whole-branch review**

Request a review of the **entire branch diff**, not the individual task diffs,
on the most capable model. Across #373, #377 and #379 the whole-branch review
found defects no task-scoped review could: a requeue-contract bug, a fourth
poison-pill shape, and an unproven cross-tenant claim. None lived inside a
single task's diff.

Ask it specifically to check three things this plan asserts but a reviewer
should not take on trust:

1. Is there any path where `Send` returns `nil` without a provider accepting
   the message? That is the single invariant the honest counters rest on.
2. Does any caller increment a `*_sent_total` counter on an error path?
3. Do the claim keys actually distinguish a legitimate day-7-after-day-5 from
   a duplicate day-5?

- [ ] **Step 7: Do NOT push, open a PR, merge, or deploy**

Report the results and wait. Deployment sends real mail to real merchants, and
the backfill must run before the first cron fires — sequencing that is the
user's call.

## Operational note for whoever ships this

Order matters on deploy:

1. Migrations 104 and 105 land.
2. `cmd/backfill-email` runs to completion (start with `--dry-run` and read the
   `Placeholder` and `NoneInStripe` counts — they are the population that will
   still get nothing).
3. Only then does the new image serve, so the first 09:05 cron has addresses to
   send to.

Running the image before the backfill is not dangerous — every row simply
counts as `skipped{reason="no_address"}` — but it burns the trial-reminder
idempotency slots for that day, and those do not come back.
