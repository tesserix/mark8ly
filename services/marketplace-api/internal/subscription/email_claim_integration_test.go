//go:build integration

package subscription_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestClaimEmailSend_SecondClaimLoses(t *testing.T) {
	db := testdb.NewDB(t, "billing_email_sends")
	ctx := context.Background()
	subID := uuid.New()
	now := time.Now().UTC()

	won, err := subscription.ClaimEmailSend(ctx, db, subID, "dunning_day_5", "2026-08-26", now)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if !won {
		t.Fatal("first claim did not win")
	}

	won, err = subscription.ClaimEmailSend(ctx, db, subID, "dunning_day_5", "2026-08-26", now)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if won {
		t.Error("second claim won — duplicate mail would be sent")
	}
}

func TestClaimEmailSend_DifferentTemplateAndPeriodBothWin(t *testing.T) {
	db := testdb.NewDB(t, "billing_email_sends")
	ctx := context.Background()
	subID := uuid.New()
	now := time.Now().UTC()

	if won, err := subscription.ClaimEmailSend(ctx, db, subID, "dunning_day_5", "2026-08-26", now); err != nil || !won {
		t.Fatalf("day_5 claim: won=%v err=%v", won, err)
	}
	// A day-7 notice after a day-5 one is legitimate, not a duplicate.
	if won, err := subscription.ClaimEmailSend(ctx, db, subID, "dunning_day_7", "2026-08-26", now); err != nil || !won {
		t.Errorf("day_7 same date should win: won=%v err=%v", won, err)
	}
	// The same template in a later period is also legitimate.
	if won, err := subscription.ClaimEmailSend(ctx, db, subID, "dunning_day_5", "2026-09-26", now); err != nil || !won {
		t.Errorf("day_5 later period should win: won=%v err=%v", won, err)
	}
}
