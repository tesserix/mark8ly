package carriersecrets

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/mark8ly/marketplace-api/internal/bao"
	"github.com/mark8ly/marketplace-api/internal/crypto"
)

// cachingTTL is the CachingStore TTL used for "bao" mode. Matches the
// value both cmd/marketplace-api and cmd/refund-sweep-cron wired inline
// before this package existed.
const cachingTTL = 60 * time.Second

// BaoClientFactory constructs an OpenBao client. Overridable in tests —
// see BuildParams.NewBaoClient.
type BaoClientFactory func(cfg bao.Config) (*bao.Client, error)

// BuildParams bundles every knob needed to construct the per-tenant
// carrier secret Store, independent of how a caller loads its config (this
// package never imports pkg/config — see the package doc).
type BuildParams struct {
	// Mode selects the backend: "inline" | "gcpsm" | "bao". Any other
	// value is an error — Build never silently coerces an unrecognised
	// mode to inline. "gcpsm" is ALWAYS an error (mark8ly#621): GCP
	// Secret Manager was retired from this package and there is no
	// client left to route it to.
	Mode string
	// OpenBaoAddr is the OpenBao API address.
	OpenBaoAddr string
	// OpenBaoMount is the OpenBao KV v2 mount name.
	OpenBaoMount string
	// OpenBaoRole is the Kubernetes auth role the OpenBao client logs in
	// as.
	OpenBaoRole string
	// Encryptor decodes legacy inline (noop:/aes:) values and backs the
	// "inline" mode store directly.
	Encryptor crypto.Encryptor
	// Logger receives the same structured log lines main.go's inline
	// switch used to emit. Nil installs a discard logger.
	Logger *slog.Logger
	// Counter feeds ChainStore's fallback-read counter and CachingStore's
	// stale-read counter. Nil installs a no-op.
	Counter CounterFn

	// NewBaoClient constructs the OpenBao client. Defaults to bao.New.
	NewBaoClient BaoClientFactory
}

// Build constructs the Store for p.Mode, preserving every behaviour of the
// mode switch this replaced in cmd/marketplace-api/main.go:
//
//   - "inline": NewInlineStore(p.Encryptor).
//   - "gcpsm": ALWAYS an error naming the mode. GCP Secret Manager was
//     retired from this package in mark8ly#621; there is no backend left
//     to build for this mode, and Build must never silently coerce it to
//     "bao" — a deployment still asking for "gcpsm" believes something
//     false about its own configuration and must fail visibly, not
//     quietly run on the wrong backend.
//   - "bao": a ChainStore with Primary: BackendBao, wrapped in a
//     CachingStore (ttl=60s).
//   - Any other failure (OpenBao client init failure, unrecognised mode)
//     is a returned error. Build never calls os.Exit — that decision
//     belongs to each caller, since main.go and the cron treat it
//     differently (main.go exits; the cron's caller may choose the same,
//     but that's the caller's call, not this package's).
func Build(ctx context.Context, p BuildParams) (Store, bool, error) {
	logger := p.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	counter := p.Counter
	if counter == nil {
		counter = func(string, int64) {}
	}
	newBaoClient := p.NewBaoClient
	if newBaoClient == nil {
		newBaoClient = bao.New
	}

	switch p.Mode {
	case "gcpsm":
		return nil, false, fmt.Errorf(
			"carriersecrets: SHIPPING_SECRET_STORE=%q is no longer supported — GCP Secret Manager was retired in mark8ly#621; set SHIPPING_SECRET_STORE to \"bao\" or \"inline\"",
			p.Mode)
	case "bao":
		baoClient, baoErr := newBaoClient(bao.Config{
			Address:        p.OpenBaoAddr,
			Mount:          p.OpenBaoMount,
			KubernetesRole: p.OpenBaoRole,
		})
		if baoErr != nil {
			return nil, false, fmt.Errorf("carriersecrets: openbao client init failed: %w", baoErr)
		}
		chain := NewChainStore(ChainConfig{
			Bao:       NewBaoClient(baoClient),
			Encryptor: p.Encryptor,
			Primary:   BackendBao,
			Counter:   counter,
			Logger:    logger,
		})
		// Caching is bao-only: gcpsm no longer exists to leave uncached,
		// but the doc comment on the call site in
		// cmd/marketplace-api/main.go still explains why this store
		// specifically wants a cache.
		store := NewCachingStore(chain, cachingTTL, time.Now, counter)
		logger.Info("carriersecrets: chain store online",
			"primary", BackendBao, "cached", true,
			"openbao_addr", p.OpenBaoAddr, "openbao_kv_mount", p.OpenBaoMount)
		return store, false, nil
	case "inline":
		logger.Info("carriersecrets: inline store (dev mode)")
		return NewInlineStore(p.Encryptor), false, nil
	default:
		return nil, false, fmt.Errorf("carriersecrets: unknown SHIPPING_SECRET_STORE mode %q", p.Mode)
	}
}
