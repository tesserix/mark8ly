package carriersecrets

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"

	"github.com/mark8ly/marketplace-api/internal/bao"
	"github.com/mark8ly/marketplace-api/internal/crypto"
)

// Backend names the credential backend a ChainStore writes new secrets to.
// It is never inferred — a ChainStore only ever writes through OpenBao. The
// type is kept (rather than collapsing ChainConfig.Primary to a bool or
// dropping it) because it is the value MaybeRewrap logs and because a
// second backend has existed here twice already (GCP Secret Manager, now
// retired — mark8ly#621) and may again; requiring an explicit, named
// Primary keeps that door open without re-deriving routing from a mode
// flag.
type Backend string

const (
	// BackendBao marks bao:// as the primary (non-fallback) reference
	// format and routes Put to OpenBao. Currently the ONLY valid value —
	// see NewChainStore.
	BackendBao Backend = "bao"
)

// FallbackReadMetric is the label passed to CounterFn when a gsm://
// reference is read. GCP Secret Manager was retired in mark8ly#621, so
// this counter no longer marks "GCP SM still has live readers" — every
// gsm:// hit now fails outright (see Get) — but it stays wired and firing:
// a non-zero count post-retirement means the carrier-secrets-backfill
// census (cmd/carrier-secrets-backfill/verify.go) missed a row, which is
// exactly the kind of miss an operator needs paged on, not silently
// swallowed into the generic "unrecognised reference" error path.
const FallbackReadMetric = "gsm_fallback_read"

// RewrapFailedMetric is the label passed to CounterFn when
// ChainStore.MaybeRewrap's write fails. The storefront engine holds no
// write grant on OpenBao by design (see the design doc's "least privilege"
// ruling), so every MaybeRewrap call on that path is expected to fail with
// bao.ErrForbidden until the active backfill (not lazy rewrap) migrates the
// row — this counter, together with the once-per-process log line at the
// forbidden transition, is what makes that failure VISIBLE instead of a
// silent no-op with a failing round-trip on every read.
const RewrapFailedMetric = "rewrap_failed"

// CounterFn is the metric injection hook, called with (label, increment)
// once per event. Kept generic — like webhookprune.CounterFn — so this
// package never imports a Prometheus client directly.
type CounterFn func(label string, increment int64)

// ChainConfig bundles the knobs for constructing a ChainStore.
type ChainConfig struct {
	// Bao is the OpenBao-backed SecretClient (usually a *BaoClient wrapping
	// Task 2's *bao.Client, or a fake in tests). Required.
	Bao SecretClient
	// Encryptor decodes legacy inline (noop:/aes:) values on read. Nil
	// disables inline-compat reads — an inline reference then errors
	// instead of silently failing to decode.
	Encryptor crypto.Encryptor
	// Primary selects which backend Put writes to. Required — must be
	// BackendBao; ChainStore panics on any other value (see
	// NewChainStore). GCP Secret Manager (BackendGCP) was retired in
	// mark8ly#621.
	Primary Backend
	// Counter receives fallback-read events. Nil installs a no-op so
	// callers that don't care about metrics (most tests) don't have to
	// pass one.
	Counter CounterFn
	// Logger receives structured log lines for events that must be
	// visible to an operator but must not fail the surrounding read path
	// — currently just MaybeRewrap failures. Nil-safe: a nil Logger
	// installs a discard logger, matching Counter's no-op convention, so
	// callers that don't care (most tests) don't have to pass one.
	Logger *slog.Logger
}

// ChainStore is the Store used for every non-inline deployment. It routes
// bao:// references to OpenBao by prefix; a gsm:// reference (a row the
// mark8ly#621 backfill missed) fails with an explicit, self-explaining
// error instead of ever reaching GCP Secret Manager — that backend was
// retired and this package holds no client for it any more.
type ChainStore struct {
	bao     SecretClient
	enc     crypto.Encryptor
	primary Backend
	counter CounterFn
	logger  *slog.Logger

	// rewrapDisabled latches to true the first time MaybeRewrap's write
	// is refused with bao.ErrForbidden. Once set, every subsequent
	// MaybeRewrap call is a no-op that skips the write attempt entirely
	// — this is what silences the per-request 403 noise on the
	// storefront path without weakening its OpenBao grant (see the
	// design doc's "least privilege" ruling: the fix is visibility and
	// stop-retrying, not a wider policy). Read/written with
	// sync/atomic so concurrent request goroutines don't race.
	rewrapDisabled atomic.Bool
}

// NewChainStore constructs a ChainStore. Panics on a missing required
// field or an unrecognised Primary — callers must pass a usable config at
// boot, exactly like NewInlineStore.
func NewChainStore(cfg ChainConfig) *ChainStore {
	if cfg.Bao == nil {
		panic("carriersecrets: ChainStore requires a Bao SecretClient")
	}
	if cfg.Primary != BackendBao {
		panic(fmt.Sprintf("carriersecrets: ChainStore requires Primary to be %q, got %q", BackendBao, cfg.Primary))
	}
	counter := cfg.Counter
	if counter == nil {
		counter = func(string, int64) {}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &ChainStore{
		bao:     cfg.Bao,
		enc:     cfg.Encryptor,
		primary: cfg.Primary,
		counter: counter,
		logger:  logger,
	}
}

// gsmRetiredError is returned for every operation against a gsm:// (GCP
// Secret Manager) reference. GCP Secret Manager was retired from this
// package in mark8ly#621: there is no client left to route the call to,
// and this error is what makes an unmigrated row diagnosable instead of
// falling through to the generic "unrecognised reference" branch, whose
// message deliberately names nothing (see referencePrefix).
func gsmRetiredError() error {
	return errors.New("carriersecrets: gsm:// reference received but GCP Secret Manager was retired (mark8ly#621); this reference cannot be resolved")
}

// Put writes plaintext to OpenBao and returns its canonical reference. On
// error it returns the error — it never falls back to another backend. A
// silent write fallback would mint gsm:// references again and make the
// mark8ly#621 decommission evidence (a durably-zero fallback counter and
// clean verify census) a lie.
func (c *ChainStore) Put(ctx context.Context, scope Scope, plaintext string) (string, error) {
	if err := scope.Validate(); err != nil {
		return "", err
	}
	path := BaoPath(scope)
	if err := c.bao.CreateOrAddVersion(ctx, path, []byte(plaintext)); err != nil {
		return "", fmt.Errorf("carriersecrets: put %s: %w", path, err)
	}
	return FormatBaoReference(scope), nil
}

// Get resolves reference to plaintext. Routing is strictly by prefix:
//
//   - "" -> ("", nil).
//   - "bao://..." -> OpenBao only.
//   - "gsm://..." -> always fails with gsmRetiredError, after counting the
//     attempt under FallbackReadMetric. GCP Secret Manager was retired in
//     mark8ly#621 — there is no backend left to route this to, and a row
//     still carrying this prefix means the backfill missed it.
//   - "noop:"/"aes:" -> the configured Encryptor.
//   - anything else -> a clear error naming the prefix. Never treated as
//     plaintext — that would hand a raw DB value to a payment gateway.
func (c *ChainStore) Get(ctx context.Context, reference string) (string, error) {
	if reference == "" {
		return "", nil
	}
	switch {
	case IsBaoRef(reference):
		path, _ := ParseBaoReference(reference)
		data, err := c.bao.AccessLatest(ctx, path)
		if err != nil {
			return "", fmt.Errorf("carriersecrets: get %s: %w", path, err)
		}
		return string(data), nil
	case IsGSMRef(reference):
		// Count the ATTEMPT: a gsm:// reference reaching this code at
		// all — whether or not it would have resolved — is evidence a
		// row still needs migrating, and that must be visible even
		// though the read errors out unconditionally now.
		c.counter(FallbackReadMetric, 1)
		return "", gsmRetiredError()
	case IsInlineRef(reference):
		if c.enc == nil {
			return "", errors.New("carriersecrets: inline reference received but no encryptor wired")
		}
		plain, err := c.enc.Decrypt(reference)
		if err != nil {
			return "", fmt.Errorf("carriersecrets: decrypt inline: %w", err)
		}
		return plain, nil
	default:
		return "", fmt.Errorf("carriersecrets: unknown reference prefix %q", referencePrefix(reference))
	}
}

// Destroy deletes the underlying secret, routed by prefix. Inline
// references (and anything else unrecognised) are a no-op — there is no
// detached resource to clean up.
func (c *ChainStore) Destroy(ctx context.Context, reference string) error {
	if reference == "" {
		return nil
	}
	switch {
	case IsBaoRef(reference):
		path, _ := ParseBaoReference(reference)
		if err := c.bao.DeleteSecret(ctx, path); err != nil {
			return fmt.Errorf("carriersecrets: destroy %s: %w", path, err)
		}
		return nil
	case IsGSMRef(reference):
		// No GCP client to route to (retired in mark8ly#621) — fail
		// loudly rather than silently reporting a delete that never
		// happened. The GCP secret this reference names still exists
		// (deleting it is a later, human-operator step) and this
		// package must never claim otherwise.
		return gsmRetiredError()
	default:
		return nil
	}
}

// MaybeRewrap migrates oldRef to the primary (bao://) format when it isn't
// already in that format. It writes through Put (so it inherits Put's
// no-fallback guarantee) and returns the new reference and true, UNLESS
// oldRef is empty, already bao://, or the rewrap write itself fails — a
// transient backend blip must not fail the surrounding read path; the next
// read tries again.
//
// oldRef being gsm:// here is only reachable if a caller already resolved
// its plaintext some other way, since Get now fails every gsm:// read
// before returning plaintext — MaybeRewrap can no longer be reached via
// the normal Get-then-rewrap flow for those rows. Migrating any remaining
// gsm:// row is the active backfill's job (cmd/carrier-secrets-backfill),
// not lazy rewrap's.
//
// The storefront engine holds no OpenBao write grant by design (least
// privilege on the internet-facing engine — see the design doc), so every
// call on that path fails Put with bao.ErrForbidden. A failure here is
// never swallowed silently: it is logged and counted under
// RewrapFailedMetric so the failure is visible instead of an inert no-op
// with a failing round-trip on every read. The FIRST time the failure is
// specifically bao.ErrForbidden, rewrapDisabled latches so every
// subsequent call on this ChainStore instance skips the write attempt
// entirely (logged once, at that transition) — this removes the
// per-request 403 noise (and OpenBao audit-log spam) without widening the
// grant. Migration for these rows is then the active backfill's job, not
// lazy rewrap's.
func (c *ChainStore) MaybeRewrap(ctx context.Context, oldRef string, scope Scope, plaintext string) (string, bool) {
	if oldRef == "" || IsBaoRef(oldRef) {
		return "", false
	}
	if c.rewrapDisabled.Load() {
		return "", false
	}
	if err := scope.Validate(); err != nil {
		return "", false
	}
	newRef, err := c.Put(ctx, scope, plaintext)
	if err != nil {
		c.counter(RewrapFailedMetric, 1)
		c.logger.Error("carriersecrets: lazy rewrap failed",
			"tenant_id", scope.TenantID, "domain", scope.Domain, "provider", scope.Provider, "field", scope.Field,
			"err", err)
		if errors.Is(err, bao.ErrForbidden) && c.rewrapDisabled.CompareAndSwap(false, true) {
			c.logger.Warn("carriersecrets: lazy rewrap disabled for this process — OpenBao refused the write; migration for remaining rows will be completed by the active backfill, not lazy rewrap")
		}
		return "", false
	}
	return newRef, true
}

// referencePrefix returns a short, non-revealing descriptor of an
// unrecognised reference for error messages. It must NEVER return the
// value itself, or any prefix of it, however long: a pre-encryption
// plaintext row (a raw gateway key with no "gsm://"/"bao://"/"noop:"/"aes:"
// wrapper) is exactly the shape of value that reaches this path, and
// callers wrap and log this error — so even a short leading slice would
// put live credential material in a log line. The length is included
// because it is diagnostically useful (e.g. distinguishing a truncated
// value from an empty one) without disclosing content.
func referencePrefix(ref string) string {
	return fmt.Sprintf("<unrecognised, len=%d>", len(ref))
}

var _ Store = (*ChainStore)(nil)
var _ Rewrapper = (*ChainStore)(nil)
