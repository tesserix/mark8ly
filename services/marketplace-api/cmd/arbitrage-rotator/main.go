// Command arbitrage-rotator rotates the HMAC key used by the anti-arbitrage
// IP hasher. Runs nightly as a Cloud Run Job triggered by Cloud Scheduler.
//
// Rotation policy (spec §18.8):
//   - New version when the newest enabled version is ≥30 days old.
//   - Old versions retained for 31 days past their replacement (so any
//     ip_hash produced under the old key can still be correlated during the
//     overlap window). After 61 days total, the old version is DISABLED.
//   - Disabled != Destroyed. A separate quarterly cleanup (follow-up)
//     destroys disabled versions.
package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	smpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"

	"github.com/mark8ly/marketplace-api/internal/arbitrage"
)

const (
	rotationAgeDays  = 30 // create a new version when newest is ≥30d old
	maxRetentionDays = 61 // disable versions older than 61 days
	hmacKeyBytes     = 32 // 256-bit CSPRNG payload
)

func main() {
	secretPath := flag.String("secret", "", "full Secret Manager resource path, e.g. projects/tesserix-prod/secrets/arbitrage-ip-hmac-key")
	flag.Parse()
	if *secretPath == "" {
		slog.Error("--secret flag is required")
		os.Exit(2)
	}

	ctx := context.Background()
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		slog.Error("secret manager client init", "err", err)
		os.Exit(1)
	}
	defer client.Close()

	if err := run(ctx, client, *secretPath); err != nil {
		slog.Error("rotation failed", "err", err)
		os.Exit(1)
	}
	slog.Info("rotation complete")
}

func run(ctx context.Context, client *secretmanager.Client, secretPath string) error {
	src := &arbitrage.SecretManagerSource{Client: client, SecretPath: secretPath}
	vs, err := src.ListEnabled(ctx)
	if err != nil {
		return fmt.Errorf("list versions: %w", err)
	}
	// Sort newest-first.
	sort.Slice(vs, func(i, j int) bool { return vs[i].CreatedAt.After(vs[j].CreatedAt) })

	if shouldRotate(vs) {
		payload := make([]byte, hmacKeyBytes)
		if _, err := rand.Read(payload); err != nil {
			return fmt.Errorf("csprng: %w", err)
		}
		_, err := client.AddSecretVersion(ctx, &smpb.AddSecretVersionRequest{
			Parent:  secretPath,
			Payload: &smpb.SecretPayload{Data: payload},
		})
		if err != nil {
			return fmt.Errorf("add version: %w", err)
		}
		slog.Info("added new key version", "secret", secretPath)
	}

	for _, old := range versionsToDisable(vs, maxRetentionDays) {
		_, err := client.DisableSecretVersion(ctx, &smpb.DisableSecretVersionRequest{
			Name: old.Name,
		})
		if err != nil {
			return fmt.Errorf("disable %s: %w", old.Name, err)
		}
		slog.Info("disabled old version", "version", old.Name, "age_days",
			int(time.Since(old.CreatedAt).Hours()/24))
	}
	return nil
}

// shouldRotate returns true when no versions exist (bootstrap) or the newest
// enabled version is at or beyond the rotation age threshold.
func shouldRotate(vs []arbitrage.KeyVersion) bool {
	if len(vs) == 0 {
		return true // first-time bootstrap
	}
	return time.Since(vs[0].CreatedAt) >= rotationAgeDays*24*time.Hour
}

// versionsToDisable returns every version older than maxDays (newest-first
// input assumed). These are disabled (not destroyed) so accidental disables
// are reversible within the subsequent 30-day window before destruction.
func versionsToDisable(vs []arbitrage.KeyVersion, maxDays int) []arbitrage.KeyVersion {
	cutoff := time.Now().Add(-time.Duration(maxDays) * 24 * time.Hour)
	var out []arbitrage.KeyVersion
	for _, v := range vs {
		if v.CreatedAt.Before(cutoff) {
			out = append(out, v)
		}
	}
	return out
}
