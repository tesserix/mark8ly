// Command fx-rate-refresh upserts mid-market FX rates into the fx_rates table
// (migration 063) so the MRRRollup Prometheus collector (internal/metrics/mrr_rollup.go)
// can convert per-currency plan prices into a unified USD MRR gauge at scrape time.
//
// # Rate source
//
// Production wiring to a live FX API (e.g. openexchangerates.org or
// exchangerate.host) is a follow-up task. For now this binary uses
// hard-coded mid-market approximations (2026 estimates) and exits 0 on
// success.
//
// # Deployment
//
// Run as a Kubernetes batch/v1 CronJob (manifest lives in tesserix-k8s).
// The binary reads DATABASE_URL from the environment, upserts rates, and exits.
// It does NOT push to a Pushgateway — the MRRRollup collector reads directly
// from the DB at scrape time.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/metrics"
	"github.com/mark8ly/marketplace-api/pkg/db"
)

// staticRates holds hard-coded mid-market approximations for currencies used
// by Mark8ly merchants (2026 estimates).
//
// TODO(p17-fx): replace with a live API call to openexchangerates.org or
// exchangerate.host once API keys are provisioned in GCP Secret Manager.
var staticRates = map[string]string{
	"USD": "1.0",
	"EUR": "1.08",
	"GBP": "1.27",
	"INR": "0.012",
	"AUD": "0.65",
	"CAD": "0.74",
	"SGD": "0.74",
	"AED": "0.27",
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Error("fx-rate-refresh: DATABASE_URL not set")
		os.Exit(1)
	}

	conn, err := db.Open(databaseURL)
	if err != nil {
		log.Error("fx-rate-refresh: db open failed", "err", err)
		os.Exit(1)
	}

	// Build rates map from the static table.
	rates := make(map[string]decimal.Decimal, len(staticRates))
	for currency, rateStr := range staticRates {
		d, parseErr := decimal.NewFromString(rateStr)
		if parseErr != nil {
			log.Error("fx-rate-refresh: invalid rate constant", "currency", currency, "value", rateStr, "err", parseErr)
			os.Exit(1)
		}
		rates[currency] = d
	}

	repo := metrics.NewFXRepository(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := repo.Upsert(ctx, rates, "static_fallback"); err != nil {
		log.Error("fx-rate-refresh: upsert failed", "err", err)
		os.Exit(1)
	}

	log.Info("fx-rate-refresh: rates upserted", "count", len(rates), "source", "static_fallback")
}
