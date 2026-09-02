package carriersecrets

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	secretmanagerclient "cloud.google.com/go/secretmanager/apiv1"

	"github.com/mark8ly/marketplace-api/internal/bao"
	"github.com/mark8ly/marketplace-api/internal/crypto"
)

// cachingTTL is the CachingStore TTL used for "bao" mode. Matches the
// value both cmd/marketplace-api and cmd/refund-sweep-cron wired inline
// before this package existed.
const cachingTTL = 60 * time.Second

// SMClientFactory constructs a GCP Secret Manager client. Overridable in
// tests so mode selection can be exercised without real GCP credentials or
// network access — see BuildParams.NewSMClient.
type SMClientFactory func(ctx context.Context) (*secretmanagerclient.Client, error)

// BaoClientFactory constructs an OpenBao client. Overridable in tests —
// see BuildParams.NewBaoClient.
type BaoClientFactory func(cfg bao.Config) (*bao.Client, error)

// BuildParams bundles every knob needed to construct the per-tenant
// carrier secret Store, independent of how a caller loads its config (this
// package never imports pkg/config — see the package doc).
type BuildParams struct {
	// Mode selects the backend: "inline" | "gcpsm" | "bao". Any other
	// value is an error — Build never silently coerces an unrecognised
	// mode to inline.
	Mode string
	// GCPProjectID is the GCP project hosting GCP SM secrets. Required
	// when Mode is "gcpsm" or "bao".
	GCPProjectID string
	// SecretPrefix is the cluster identifier prefixed onto every GCP SM
	// secret ID. Required alongside GCPProjectID.
	SecretPrefix string
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

	// NewSMClient constructs the GCP Secret Manager client. Defaults to
	// secretmanagerclient.NewClient. Tests inject a fake (or
	// error-returning) factory to exercise "gcpsm"/"bao" mode selection
	// without a real network call.
	NewSMClient SMClientFactory
	// NewBaoClient constructs the OpenBao client. Defaults to bao.New.
	NewBaoClient BaoClientFactory
}

// Build constructs the Store for p.Mode, preserving every behaviour of the
// mode switch this replaced in cmd/marketplace-api/main.go:
//
//   - "inline": NewInlineStore(p.Encryptor).
//   - "gcpsm": a ChainStore with Primary: BackendGCP, UNCACHED. Caching
//     this would add stale-on-error masking to the path nobody asked to
//     change — see the switch's original doc comment, preserved on the
//     call site in cmd/marketplace-api/main.go.
//   - "bao": a ChainStore with Primary: BackendBao, wrapped in a
//     CachingStore (ttl=60s).
//   - A GCP Secret Manager client that fails to construct in "gcpsm" or
//     "bao" mode is NOT an error — it degrades to NewInlineStore and
//     reports degraded=true, exactly as main.go did.
//   - Any other failure (missing GCPProjectID, OpenBao client init
//     failure, unrecognised mode) is a returned error. Build never calls
//     os.Exit — that decision belongs to each caller, since main.go and
//     the cron treat it differently (main.go exits; the cron's caller
//     may choose the same, but that's the caller's call, not this
//     package's).
func Build(ctx context.Context, p BuildParams) (Store, bool, error) {
	logger := p.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	counter := p.Counter
	if counter == nil {
		counter = func(string, int64) {}
	}
	newSMClient := p.NewSMClient
	if newSMClient == nil {
		newSMClient = func(ctx context.Context) (*secretmanagerclient.Client, error) {
			return secretmanagerclient.NewClient(ctx)
		}
	}
	newBaoClient := p.NewBaoClient
	if newBaoClient == nil {
		newBaoClient = bao.New
	}

	switch p.Mode {
	case "gcpsm", "bao":
		if p.GCPProjectID == "" {
			return nil, false, fmt.Errorf(
				"carriersecrets: GCP_PROJECT_ID required when SHIPPING_SECRET_STORE=%s", p.Mode)
		}
		smClient, smErr := newSMClient(ctx)
		if smErr != nil {
			logger.Error("carriersecrets: secret manager client init failed — falling back to inline",
				"err", smErr)
			return NewInlineStore(p.Encryptor), true, nil
		}
		baoClient, baoErr := newBaoClient(bao.Config{
			Address:        p.OpenBaoAddr,
			Mount:          p.OpenBaoMount,
			KubernetesRole: p.OpenBaoRole,
		})
		if baoErr != nil {
			return nil, false, fmt.Errorf("carriersecrets: openbao client init failed: %w", baoErr)
		}
		primary := BackendGCP
		if p.Mode == "bao" {
			primary = BackendBao
		}
		chain := NewChainStore(ChainConfig{
			Bao:          NewBaoClient(baoClient),
			GCP:          NewGCPStore(smClient, p.GCPProjectID),
			Encryptor:    p.Encryptor,
			Primary:      primary,
			GCPProjectID: p.GCPProjectID,
			GCPPrefix:    p.SecretPrefix,
			Counter:      counter,
			Logger:       logger,
		})
		var store Store
		cached := p.Mode == "bao"
		if cached {
			// Caching is bao-only — see the doc comment above and on
			// the call site in cmd/marketplace-api/main.go for why
			// "gcpsm" stays uncached.
			store = NewCachingStore(chain, cachingTTL, time.Now, counter)
		} else {
			store = chain
		}
		logger.Info("carriersecrets: chain store online",
			"primary", primary, "cached", cached,
			"project_id", p.GCPProjectID, "prefix", p.SecretPrefix,
			"openbao_addr", p.OpenBaoAddr, "openbao_kv_mount", p.OpenBaoMount)
		return store, false, nil
	case "inline":
		logger.Info("carriersecrets: inline store (dev mode)")
		return NewInlineStore(p.Encryptor), false, nil
	default:
		return nil, false, fmt.Errorf("carriersecrets: unknown SHIPPING_SECRET_STORE mode %q", p.Mode)
	}
}
