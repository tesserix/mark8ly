// Command break-glass-rotation rotates per-tenant break-glass admin
// credentials. Two triggers share a pass (§12.4):
//
//   - Post-use: every successful /admin/break-glass/login stamps
//     rotation_scheduled_at = now+24h. The next cron pass rotates
//     those rows.
//   - 90-day cadence: any account with last_rotated_at older than 90
//     days is rotated regardless of post-use state.
//
// Both triggers land on rotator.RotateDue, which runs a single pass
// and never rolls back partial failures — one dead tenant must not
// stop the others. Intended to run as a Kubernetes CronJob daily at
// 04:00 UTC.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/bao"
	"github.com/mark8ly/marketplace-api/internal/breakglass"
	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
	"github.com/mark8ly/marketplace-api/pkg/config"
	"github.com/mark8ly/marketplace-api/pkg/db"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// LoadCarrierSecretJob reads only DATABASE_URL / SHIPPING_SECRET_STORE /
	// OPENBAO_ADDR / OPENBAO_ROLE / OPENBAO_KV_MOUNT / encryption fields —
	// NOT the full config.Load(), which requires MARKETPLACE_FGA_API_URL
	// unconditionally and, outside ENV=dev, secrets this job never touches.
	// It's the same loader cmd/refund-sweep-cron uses, and the OpenBao
	// client it configures is the same production credential store
	// break-glass secrets now live in (mark8ly#621 retired GCP Secret
	// Manager; that backend and this job's use of it are gone as of #642).
	cfg, err := config.LoadCarrierSecretJob()
	if err != nil {
		log.Error("break-glass-rotation: config load failed", "err", err)
		os.Exit(1)
	}

	conn, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Error("break-glass-rotation: db open failed", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	baoClient, err := bao.New(bao.Config{
		Address:        cfg.OpenBaoAddr,
		Mount:          cfg.OpenBaoKVMount,
		KubernetesRole: cfg.OpenBaoRole,
	})
	if err != nil {
		log.Error("break-glass-rotation: openbao client init failed", "err", err)
		os.Exit(1)
	}

	repo := breakglass.NewRepository(conn)
	secrets := breakglass.NewSecretManager(breakglass.NewBaoSecretClient(carriersecrets.NewBaoClient(baoClient)))

	// Audit emitter — fire-and-forget. nil is safe downstream.
	auditRepo := audit.NewRepository()
	auditEmitter, err := audit.NewEmitter(audit.EmitterConfig{
		DB:     conn,
		Repo:   auditRepo,
		Logger: log,
	})
	if err != nil {
		log.Error("break-glass-rotation: audit emitter init failed", "err", err)
		os.Exit(1)
	}

	// Slack — optional. Local dev without a webhook silently no-ops.
	slack := breakglass.NewSlackClient(
		os.Getenv("SLACK_SECURITY_ALERTS_WEBHOOK"),
		breakglass.SlackChannel,
	)

	// IP HMAC key — rotation emits non-IP events, but the audit
	// emitter's constructor needs a key. An empty key still produces
	// valid HMACs; the value just isn't correlatable with live login
	// logs (acceptable because rotation events don't include IPs).
	hmacKey := breakglass.HMACKey(os.Getenv("BREAK_GLASS_IP_HMAC_KEY"))
	auditE := breakglass.NewAuditEmitter(auditEmitter, hmacKey)

	rotator := breakglass.NewRotator(repo, secrets, auditE, slack)

	rotated, err := rotator.RotateDue(ctx)
	if err != nil {
		log.Error("break-glass-rotation: run failed", "err", err)
		drain(auditEmitter)
		os.Exit(1)
	}

	log.Info("break-glass-rotation: done", "rotated", rotated)
	drain(auditEmitter)
}

func drain(e *audit.Emitter) {
	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	e.Stop(drainCtx)
}
