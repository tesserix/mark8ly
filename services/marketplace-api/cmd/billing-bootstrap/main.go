// Package main is the Stripe bootstrap CLI. Run once per environment (or whenever
// the pricing catalog changes) to push Products + Prices into Stripe. Safe to re-run.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/mark8ly/marketplace-api/cmd/billing-bootstrap/bootstrap"
	"github.com/mark8ly/marketplace-api/internal/billing/pricing"
	stripec "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

func main() {
	var apiKey string
	flag.StringVar(&apiKey, "api-key", os.Getenv("STRIPE_BILLING_SECRET_KEY"), "Stripe secret key (or set STRIPE_BILLING_SECRET_KEY)")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if apiKey == "" {
		logger.Error("STRIPE_BILLING_SECRET_KEY or -api-key required")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := bootstrap.Run(ctx, stripec.New(apiKey), pricing.AllDescriptors(), logger); err != nil {
		logger.Error("bootstrap failed", "err", err)
		os.Exit(1)
	}
	logger.Info("bootstrap complete")
}
