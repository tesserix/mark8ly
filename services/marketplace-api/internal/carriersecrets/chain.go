package carriersecrets

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"

	"github.com/mark8ly/marketplace-api/internal/bao"
	"github.com/mark8ly/marketplace-api/internal/crypto"
)

// Backend names one of the two credential backends a ChainStore can route
// to. It is never inferred from a mode flag — routing is always by
// reference prefix (see ChainStore.Get/Destroy) — Backend only says which
// backend Put writes to and which format counts as "primary" for
// MaybeRewrap and the fallback counter.
type Backend string

const (
	// BackendBao routes Put to OpenBao and marks bao:// as the primary
	// (non-fallback) reference format.
	BackendBao Backend = "bao"
	// BackendGCP routes Put to GCP Secret Manager and marks gsm:// as the
	// primary (non-fallback) reference format.
	BackendGCP Backend = "gcp"
)

// FallbackReadMetric is the label passed to CounterFn when a gsm://
// reference is read while OpenBao is primary. It is the migration's only
// evidence that GCP Secret Manager still has live readers — phase 5 decides
// whether GCP SM can be decommissioned by watching this counter stay at
// zero for N days. Never fired for a bao:// read.
const FallbackReadMetric = "carriersecrets_gsm_fallback_read"

// RewrapFailedMetric is the label passed to CounterFn when
// ChainStore.MaybeRewrap's write fails. The storefront engine holds no
// write grant on OpenBao by design (see the design doc's "least privilege"
// ruling), so every MaybeRewrap call on that path is expected to fail with
// bao.ErrForbidden until the active backfill (not lazy rewrap) migrates the
// row — this counter, together with the once-per-process log line at the
// forbidden transition, is what makes that failure VISIBLE instead of a
// silent no-op with a failing round-trip on every read.
const RewrapFailedMetric = "carriersecrets_rewrap_failed"

// CounterFn is the metric injection hook, called with (label, increment)
// once per event. Kept generic — like webhookprune.CounterFn — so this
// package never imports a Prometheus client directly.
type CounterFn func(label string, increment int64)

// ChainConfig bundles the knobs for constructing a ChainStore.
type ChainConfig struct {
	// Bao is the OpenBao-backed SecretClient (usually a *BaoClient wrapping
	// Task 2's *bao.Client, or a fake in tests). Required.
	Bao SecretClient
	// GCP is the GCP SM-backed SecretClient (usually *GCPStore, or a fake
	// in tests). Required.
	GCP SecretClient
	// Encryptor decodes legacy inline (noop:/aes:) values on read. Nil
	// disables inline-compat reads — an inline reference then errors
	// instead of silently failing to decode.
	Encryptor crypto.Encryptor
	// Primary selects which backend Put writes to, and which reference
	// format counts as "already migrated" for MaybeRewrap and the
	// fallback counter. Required — must be BackendBao or BackendGCP.
	Primary Backend
	// GCPProjectID is the GCP project hosting GCP SM secrets. Required —
	// used to build the gsm:// reference/resource path regardless of
	// which backend is primary, since a bao-primary chain still needs to
	// read and destroy pre-cutover gsm:// rows.
	GCPProjectID string
	// GCPPrefix is the cluster identifier prefixed onto every GCP SM
	// secret ID (see SecretName). Required, same reason as GCPProjectID.
	GCPPrefix string
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

// ChainStore is the Store that lets bao:// and gsm:// references coexist
// during the OpenBao migration. Routing is always by the reference's
// prefix, never by which backend is "primary" — Primary only decides where
// Put writes and what MaybeRewrap upgrades toward.
type ChainStore struct {
	bao          SecretClient
	gcp          SecretClient
	enc          crypto.Encryptor
	primary      Backend
	gcpProjectID string
	gcpPrefix    string
	counter      CounterFn
	logger       *slog.Logger

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
// boot, exactly like NewHybridStore.
func NewChainStore(cfg ChainConfig) *ChainStore {
	if cfg.Bao == nil {
		panic("carriersecrets: ChainStore requires a Bao SecretClient")
	}
	if cfg.GCP == nil {
		panic("carriersecrets: ChainStore requires a GCP SecretClient")
	}
	if cfg.Primary != BackendBao && cfg.Primary != BackendGCP {
		panic(fmt.Sprintf("carriersecrets: ChainStore requires Primary to be %q or %q, got %q", BackendBao, BackendGCP, cfg.Primary))
	}
	if cfg.GCPProjectID == "" {
		panic("carriersecrets: ChainStore requires GCPProjectID")
	}
	if cfg.GCPPrefix == "" {
		panic("carriersecrets: ChainStore requires GCPPrefix")
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
		bao:          cfg.Bao,
		gcp:          cfg.GCP,
		enc:          cfg.Encryptor,
		primary:      cfg.Primary,
		gcpProjectID: cfg.GCPProjectID,
		gcpPrefix:    cfg.GCPPrefix,
		counter:      counter,
		logger:       logger,
	}
}

// Put writes plaintext to the primary backend ONLY and returns its
// canonical reference. On error it returns the error — it never falls back
// to the other backend. A silent write fallback would mint gsm:// (or
// bao://) references after cutover and make the fallback-read counter a
// lie, which is the sole evidence phase 5 uses to decide GCP SM is safe to
// decommission.
func (c *ChainStore) Put(ctx context.Context, scope Scope, plaintext string) (string, error) {
	if err := scope.Validate(); err != nil {
		return "", err
	}
	switch c.primary {
	case BackendBao:
		path := BaoPath(scope)
		if err := c.bao.CreateOrAddVersion(ctx, path, []byte(plaintext)); err != nil {
			return "", fmt.Errorf("carriersecrets: put %s: %w", path, err)
		}
		return FormatBaoReference(scope), nil
	default: // BackendGCP
		resource := SecretResource(c.gcpProjectID, c.gcpPrefix, scope)
		if err := c.gcp.CreateOrAddVersion(ctx, resource, []byte(plaintext)); err != nil {
			return "", fmt.Errorf("carriersecrets: put %s: %w", resource, err)
		}
		return GSMRefPrefix + resource, nil
	}
}

// Get resolves reference to plaintext. Routing is strictly by prefix:
//
//   - "" -> ("", nil), matching HybridStore.
//   - "bao://..." -> OpenBao only. Never falls back to GCP on error — the
//     value simply isn't there, and falling back would turn a transient
//     OpenBao failure into a confusing not-found against the wrong backend.
//   - "gsm://..." -> GCP SM. Increments the fallback counter when OpenBao
//     is primary (a gsm:// row read while primary=bao is exactly the
//     "still depends on GCP SM" signal phase 5 needs).
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
		resource, _ := ParseReference(reference)
		data, err := c.gcp.AccessLatest(ctx, resource)
		if err != nil {
			return "", fmt.Errorf("carriersecrets: get %s: %w", resource, err)
		}
		if c.primary == BackendBao {
			c.counter(FallbackReadMetric, 1)
		}
		return string(data), nil
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
// references (and anything else unrecognised) are a no-op, matching
// HybridStore — there is no detached resource to clean up.
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
		resource, _ := ParseReference(reference)
		if err := c.gcp.DeleteSecret(ctx, resource); err != nil {
			return fmt.Errorf("carriersecrets: destroy %s: %w", resource, err)
		}
		return nil
	default:
		return nil
	}
}

// MaybeRewrap migrates oldRef toward the CURRENT primary backend's format,
// in whichever direction that is — it is symmetric, not one-way. Under
// Bao-primary that upgrades gsm:// -> bao://; under GCP-primary it moves
// bao:// -> gsm://. This is deliberate and it is what makes rollback safe:
// if a deployment flips SHIPPING_SECRET_STORE (or the equivalent config)
// back from baokv to gcpsm after some rows have already migrated to
// OpenBao, MaybeRewrap does not need special-casing to undo that — the very
// next save of a bao:// row under GCP-primary rewraps it back into GCP
// Secret Manager, gradually bringing rows home. Reads keep working
// throughout regardless of which format a given row is in, because Get
// routes by the reference's own prefix, never by which backend is primary.
//
// Concretely: it writes through Put (so it inherits Put's no-fallback
// guarantee) and returns the new reference and true, UNLESS oldRef is
// already in the primary's format, in which case it is a no-op. Returns
// ("", false) when oldRef is empty, already in the primary's format, or the
// rewrap write itself fails — a transient backend blip must not fail the
// surrounding read path; the next read tries again.
//
// The storefront engine holds no OpenBao write grant by design (least
// privilege on the internet-facing engine — see the design doc), so under
// Bao-primary every call on that path fails Put with bao.ErrForbidden. A
// failure here is never swallowed silently: it is logged and counted under
// RewrapFailedMetric so the failure is visible instead of an inert no-op
// with a failing round-trip on every read. The FIRST time the failure is
// specifically bao.ErrForbidden, rewrapDisabled latches so every
// subsequent call on this ChainStore instance skips the write attempt
// entirely (logged once, at that transition) — this removes the
// per-request 403 noise (and OpenBao audit-log spam) without widening the
// grant. Migration for these rows is then the active backfill's job, not
// lazy rewrap's.
func (c *ChainStore) MaybeRewrap(ctx context.Context, oldRef string, scope Scope, plaintext string) (string, bool) {
	if oldRef == "" {
		return "", false
	}
	alreadyPrimary := (c.primary == BackendBao && IsBaoRef(oldRef)) || (c.primary == BackendGCP && IsGSMRef(oldRef))
	if alreadyPrimary {
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

// referencePrefix extracts a human-readable prefix from an unrecognised
// reference for error messages — the scheme portion up to "://" when
// present, else the portion up to the first ':', else the whole value.
func referencePrefix(ref string) string {
	if idx := strings.Index(ref, "://"); idx >= 0 {
		return ref[:idx+3]
	}
	if idx := strings.Index(ref, ":"); idx >= 0 {
		return ref[:idx+1]
	}
	return ref
}

var _ Store = (*ChainStore)(nil)
var _ Rewrapper = (*ChainStore)(nil)
