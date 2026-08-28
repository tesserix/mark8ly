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

	secretmanager "cloud.google.com/go/secretmanager/apiv1"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/breakglass"
	"github.com/mark8ly/marketplace-api/pkg/db"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Error("break-glass-rotation: DATABASE_URL not set")
		os.Exit(1)
	}

	conn, err := db.Open(databaseURL)
	if err != nil {
		log.Error("break-glass-rotation: db open failed", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	smClient, err := secretmanager.NewClient(ctx)
	if err != nil {
		log.Error("break-glass-rotation: secret manager client init failed", "err", err)
		os.Exit(1)
	}
	defer smClient.Close()

	repo := breakglass.NewRepository(conn)
	secrets := breakglass.NewSecretManager(breakglass.NewGCPSecretClient(smClient))

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
