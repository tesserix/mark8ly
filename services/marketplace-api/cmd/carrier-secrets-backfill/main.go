package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/metrics"
	"github.com/mark8ly/marketplace-api/pkg/config"
	"github.com/mark8ly/marketplace-api/pkg/db"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	dryRun := flag.Bool("dry-run", true,
		"report what would migrate without writing anything to the Store or the DB. "+
			"Default true — pass -dry-run=false to actually write.")
	verify := flag.Bool("verify", false,
		"read every stored reference through the Store and report whether each "+
			"resolves, plus a census by reference scheme. Never writes. Used by "+
			"mark8ly#621 to prove the credential paths were actually exercised "+
			"before GCP Secret Manager is deleted.")
	flag.Parse()

	// LoadCarrierSecretJob mirrors cmd/refund-sweep-cron: it reads only
	// DATABASE_URL / SHIPPING_SECRET_STORE / GCP_PROJECT_ID /
	// SECRET_NAME_PREFIX / OPENBAO_ADDR / OPENBAO_ROLE / OPENBAO_KV_MOUNT /
	// ENCRYPTION_MODE / ENCRYPTION_KEY, not the full config.Load() — this
	// job never touches FGA, internal auth, or customer sessions.
	cfg, err := config.LoadCarrierSecretJob()
	if err != nil {
		log.Error("carrier-secrets-backfill: config load failed", "err", err)
		os.Exit(1)
	}

	// Running this job under any mode other than "bao" cannot produce
	// bao:// references — see requireBaoPrimary's doc comment.
	if err := requireBaoPrimary(cfg.ShippingSecretStore); err != nil {
		log.Error("carrier-secrets-backfill: refusing to run", "err", err)
		os.Exit(1)
	}

	conn, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Error("carrier-secrets-backfill: db open failed", "err", err)
		os.Exit(1)
	}

	// Encryptor for legacy inline (noop:/aes:) references the Store may
	// still need to decode on read — mirrors cmd/refund-sweep-cron and
	// cmd/marketplace-api's construction exactly.
	var apiKeyEncryptor crypto.Encryptor
	switch cfg.EncryptionMode {
	case "aes":
		if cfg.EncryptionKey == "" {
			log.Error("carrier-secrets-backfill: ENCRYPTION_KEY required when ENCRYPTION_MODE=aes")
			os.Exit(1)
		}
		key, decodeErr := crypto.DecodeKey(cfg.EncryptionKey)
		if decodeErr != nil {
			log.Error("carrier-secrets-backfill: invalid ENCRYPTION_KEY", "err", decodeErr)
			os.Exit(1)
		}
		enc, aesErr := crypto.NewAESEncryptor(key)
		if aesErr != nil {
			log.Error("carrier-secrets-backfill: create AES encryptor failed", "err", aesErr)
			os.Exit(1)
		}
		apiKeyEncryptor = enc
	default:
		apiKeyEncryptor = crypto.NewNoopEncryptor()
	}

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
		log.Error("carrier-secrets-backfill: carrier secret store build failed", "err", buildErr)
		os.Exit(1)
	}
	// A store degraded to InlineStore cannot resolve gsm:// references
	// (it errors loudly instead) and cannot write bao:// ones either —
	// running the backfill against it would fail every row. Fatal here,
	// same reasoning as cmd/refund-sweep-cron.
	if degraded {
		log.Error("carrier-secrets-backfill: carrier secret store degraded to inline — refusing to run a backfill that cannot resolve gsm:// or write bao:// references",
			"shipping_secret_store", cfg.ShippingSecretStore)
		os.Exit(1)
	}

	b := &Backfiller{
		Rows:   newGormRowStore(conn),
		Store:  secretStore,
		DryRun: *dryRun,
		Logger: log,
	}

	if *verify {
		vres, verr := b.Verify(context.Background())
		if verr != nil {
			log.Error("carrier-secrets-backfill: verify failed", "err", verr)
			os.Exit(1)
		}
		log.Info("carrier-secrets-backfill: verify done",
			"examined", vres.Examined,
			"resolved", vres.Resolved,
			"failed", vres.Failed,
			"by_scheme", vres.ByScheme)
		if vres.Failed > 0 {
			os.Exit(1)
		}
		return
	}

	res, err := b.Run(context.Background())
	if err != nil {
		log.Error("carrier-secrets-backfill: run failed", "err", err)
		os.Exit(1)
	}

	log.Info("carrier-secrets-backfill: done",
		"dry_run", *dryRun,
		"examined", res.Examined,
		"skipped", res.Skipped,
		"migrated", res.Migrated,
		"failed", res.Failed)

	if res.Failed > 0 {
		os.Exit(1)
	}
}
