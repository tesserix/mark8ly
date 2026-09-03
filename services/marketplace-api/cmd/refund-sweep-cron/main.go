// Command refund-sweep-cron re-drives refund_transactions rows stuck in
// 'pending' — the never-lost guarantee for refunds. A row is stuck when the
// gateway call succeeded but the process crashed (or the DB blipped) before
// the finalize transaction committed. The sweeper re-calls the gateway with
// the SAME idempotency key (a provider no-op if the money already moved)
// and completes the DB finalize + bookkeeping.
//
// Designed to run as a Cloud Run Job on a Cloud Scheduler trigger (every 5
// min). Exits non-zero only on infrastructure failures (DB connection,
// carrier secret store construction, etc.) — a sweep that resumes zero
// rows is a normal, successful run.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/metrics"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/orderrefund"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/internal/payment"
	"github.com/mark8ly/marketplace-api/pkg/config"
	"github.com/mark8ly/marketplace-api/pkg/db"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// LoadCarrierSecretJob reads only DATABASE_URL / SHIPPING_SECRET_STORE
	// / GCP_PROJECT_ID / SECRET_NAME_PREFIX / OPENBAO_ADDR / OPENBAO_ROLE
	// / OPENBAO_KV_MOUNT / ENCRYPTION_MODE / ENCRYPTION_KEY — NOT the full
	// config.Load(), which requires MARKETPLACE_FGA_API_URL
	// unconditionally and, outside ENV=dev, the internal-auth and
	// customer-session secrets too. This job never touches FGA, internal
	// auth, or customer sessions; loading the full Config would crash-loop
	// it until its deployment manifest grew settings it never reads, and
	// would widen the blast radius of secrets it never touches. It DOES
	// go through the same validateShippingSecretStore() the API uses (see
	// pkg/config.LoadCarrierSecretJob), so a misconfigured
	// SHIPPING_SECRET_STORE fails the cron at boot exactly as it would
	// fail the API.
	cfg, err := config.LoadCarrierSecretJob()
	if err != nil {
		log.Error("refund-sweep-cron: config load failed", "err", err)
		os.Exit(1)
	}

	conn, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Error("refund-sweep-cron: db open failed", "err", err)
		os.Exit(1)
	}

	// API key encryptor — AES-256-GCM in production, noop in dev. Mirrors
	// cmd/marketplace-api/main.go's construction: the same
	// ENCRYPTION_MODE/ENCRYPTION_KEY pair backs both the inline-mode
	// carrier secret store and the Resolver's inline-ciphertext fallback.
	var apiKeyEncryptor crypto.Encryptor
	switch cfg.EncryptionMode {
	case "aes":
		if cfg.EncryptionKey == "" {
			log.Error("refund-sweep-cron: ENCRYPTION_KEY required when ENCRYPTION_MODE=aes")
			os.Exit(1)
		}
		key, decodeErr := crypto.DecodeKey(cfg.EncryptionKey)
		if decodeErr != nil {
			log.Error("refund-sweep-cron: invalid ENCRYPTION_KEY", "err", decodeErr)
			os.Exit(1)
		}
		enc, aesErr := crypto.NewAESEncryptor(key)
		if aesErr != nil {
			log.Error("refund-sweep-cron: create AES encryptor failed", "err", aesErr)
			os.Exit(1)
		}
		apiKeyEncryptor = enc
	default:
		apiKeyEncryptor = crypto.NewNoopEncryptor()
	}

	// Carrier secret store — shares internal/carriersecrets.Build with
	// cmd/marketplace-api so the two callers can never again drift the
	// way that produced mark8ly#166: the cron built no store at all
	// (never called WithSecretStore), so GatewayFor could not resolve
	// the gsm:// credential references on payment_gateway_configs and
	// handed the raw reference to the gateway, which 401'd — every
	// gateway re-drive from the sweeper failed. A store-construction
	// failure here is fatal (unlike the API, which degrades to inline on
	// a Secret Manager client failure but still exits on every other
	// error): resolving credentials is the entire point of this job, so
	// a build that can't produce a working store must not run a sweep
	// that will just fail every row's gateway call again.
	secretStore, degraded, buildErr := carriersecrets.Build(context.Background(), carriersecrets.BuildParams{
		Mode:         cfg.ShippingSecretStore,
		OpenBaoAddr:  cfg.OpenBaoAddr,
		OpenBaoMount: cfg.OpenBaoKVMount,
		OpenBaoRole:  cfg.OpenBaoRole,
		Encryptor:    apiKeyEncryptor,
		Logger:       log,
		Counter:      metrics.CarrierSecretCounter,
	})
	if buildErr != nil {
		log.Error("refund-sweep-cron: carrier secret store build failed", "err", buildErr, "shipping_secret_store", cfg.ShippingSecretStore)
		os.Exit(1)
	}
	// carriersecrets.Build degrades to an InlineStore (degraded=true, no
	// error) when the configured mode is "gcpsm"/"bao" but the Secret
	// Manager client failed to construct — the API is allowed to run in
	// that state because an InlineStore still errors loudly on a gsm://
	// reference rather than passing it through (it just can't be
	// resolved). But for THIS job, degraded in a non-"inline" mode means
	// the sweep would run and fail every row needing a gsm:// or bao://
	// credential — the exact failure mode mark8ly#166 was about. Treat
	// it as fatal here instead of proceeding to a sweep that cannot
	// possibly succeed. In "inline" mode, Build never sets degraded=true
	// (see build.go: the degrade path is only reachable from the
	// "gcpsm"/"bao" branch), so this check is unreachable there — an
	// inline configuration never needs a Secret Manager client.
	if degraded && cfg.ShippingSecretStore != "inline" {
		log.Error("refund-sweep-cron: carrier secret store degraded to inline, refusing to sweep with a store that cannot resolve gsm:// or bao:// references",
			"shipping_secret_store", cfg.ShippingSecretStore)
		os.Exit(1)
	}

	res := orderrefund.NewResolver(conn).WithSecretStore(secretStore).WithEncryptor(apiKeyEncryptor)
	pay := payment.NewService(payment.NewRepository(conn))
	orders := order.NewService(conn, order.NewRepository(), outbox.NewRepository(conn))
	// enabled=true: the sweep is itself the recovery path for the
	// REFUND_GATEWAY_ENABLED kill switch — a stuck pending row must still be
	// re-driven so a later flip back to enabled doesn't leave it orphaned.
	// WithLogger is what makes the sweep's per-row skips visible (#169).
	// Without it ResumePending drops every skip silently and a persistently
	// stuck refund looks identical to a clean run reporting resumed=0.
	coord := orderrefund.NewCoordinator(conn, res, pay, orders, order.NewRepository(), true).
		WithLogger(log)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	n, err := coord.ResumePending(ctx, 5*time.Minute, 200)
	if err != nil {
		log.Error("refund-sweep-cron: run failed", "err", err)
		os.Exit(1)
	}

	log.Info("refund-sweep-cron: done", "resumed", n)
}
