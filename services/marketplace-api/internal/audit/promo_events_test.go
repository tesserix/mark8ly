package audit_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

// emitPromoAndDrain emits one promo_applied event and returns the row that
// was written. Stop drains the queue, so this is deterministic rather than an
// "eventually" assertion against a background writer.
func emitPromoAndDrain(t *testing.T, p audit.PromoApplied) *audit.Entry {
	t.Helper()
	repo := &recordingRepo{}
	e, err := audit.NewEmitter(audit.EmitterConfig{DB: nil, Repo: repo, Logger: slog.Default()})
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	e.EmitPromoApplied(c, p)
	e.Stop(context.Background())

	require.Len(t, repo.created, 1)
	return repo.created[0]
}

// A promo can move a merchant's billing date, which is the same consequential
// write an operator extension emits its own audit row for. The code string
// alone does not record it: the definition behind it lives in the console and
// can be edited afterwards (#620).
func TestEmitPromoApplied_RecordsTheTrialExtensionItGranted(t *testing.T) {
	end := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	entry := emitPromoAndDrain(t, audit.PromoApplied{
		TenantID:           uuid.New(),
		StoreID:            uuid.New(),
		Code:               "STAYLONGER",
		Actor:              "user:" + uuid.New().String(),
		Accepted:           true,
		TrialExtensionDays: 14,
		TrialEndsAt:        end,
	})

	require.Equal(t, float64(14), toNumber(t, entry.Metadata["trial_extension_days"]))
	require.Equal(t, "2026-11-03T00:00:00Z", entry.Metadata["trial_ends_at"])
}

// A discount-only code granted no days, and the row must not imply it did.
// A zero would read as "extended by nothing" rather than "not that kind of
// code", and a zero-valued date would render as the year 1.
func TestEmitPromoApplied_OmitsTheTrialFieldsWhenNoDaysWereGranted(t *testing.T) {
	entry := emitPromoAndDrain(t, audit.PromoApplied{
		TenantID: uuid.New(),
		StoreID:  uuid.New(),
		Code:     "WINBACK20OFF6MONTHS",
		Actor:    "system:winback",
		Accepted: true,
	})

	require.NotContains(t, entry.Metadata, "trial_extension_days")
	require.NotContains(t, entry.Metadata, "trial_ends_at")
}

// toNumber tolerates the int/float64 ambiguity a metadata map picks up when
// it round-trips through JSON, so the assertion is about the VALUE rather
// than about which side of a serialiser the test happens to sit on.
func toNumber(t *testing.T, v any) float64 {
	t.Helper()
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case float64:
		return n
	default:
		t.Fatalf("metadata value %v (%T) is not a number", v, v)
		return 0
	}
}
