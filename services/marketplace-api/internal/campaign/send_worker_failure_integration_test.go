//go:build integration

package campaign

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

type alwaysFailingDispatcher struct{ calls int }

func (d *alwaysFailingDispatcher) Send(context.Context, OutboundEmail) error {
	d.calls++
	return errors.New("provider refused the message")
}

type noThemeLoader struct{}

func (noThemeLoader) LoadTheme(context.Context, uuid.UUID) (CampaignTheme, error) {
	return CampaignTheme{}, nil
}

func seedCampaignWithRecipients(t *testing.T, db *gorm.DB, n int) Campaign {
	t.Helper()
	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, db, tenantID, storeID)

	c := Campaign{
		ID: uuid.New(), TenantID: tenantID, StoreID: storeID,
		Name: "348c", Type: TypeEmail, Status: StatusSending,
		Subject: strPtr("hello"), Content: strPtr("<p>hi</p>"),
	}
	require.NoError(t, db.Create(&c).Error)

	for i := 0; i < n; i++ {
		r := CampaignRecipient{
			ID: uuid.New(), CampaignID: c.ID,
			CustomerEmail: uuid.NewString() + "@example.com",
			Status:        RecipientPending,
		}
		require.NoError(t, db.Create(&r).Error)
	}
	return c
}

func recipientStatuses(t *testing.T, db *gorm.DB, campaignID uuid.UUID) map[string]int {
	t.Helper()
	var rows []CampaignRecipient
	require.NoError(t, db.Where("campaign_id = ?", campaignID).Find(&rows).Error)
	out := map[string]int{}
	for _, r := range rows {
		out[r.Status]++
	}
	return out
}

// #348C: a recipient whose dispatch permanently fails was left at `pending`.
//
// That is not merely a reporting gap. GetPendingRecipients selects
// `status = 'pending'`, and dispatchCampaign loops until a batch comes back
// EMPTY — so a permanently-failing recipient is re-fetched forever, and the
// campaign budget is re-reserved on every pass. One bad address could burn a
// merchant's entire monthly email cap before ErrBudgetExhausted paused the
// campaign; with no budget reserver wired, nothing stopped it at all.
//
// The bounded context is the assertion: before the fix this test does not
// fail, it HANGS.
func TestIntegration_SendWorker_PermanentFailureIsTerminalNotRetriedForever(t *testing.T) {
	db := testdb.NewTx(t)
	c := seedCampaignWithRecipients(t, db, 3)
	disp := &alwaysFailingDispatcher{}

	w := NewSendWorker(SendWorkerConfig{
		DB: db, Repo: NewRepository(db), Dispatcher: disp,
		ThemeLoader: noThemeLoader{},
		Logger:      slog.New(slog.NewTextHandler(nopWriter{}, nil)),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- w.dispatchCampaign(ctx, c) }()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("dispatchCampaign did not terminate: failed recipients stayed 'pending' "+
			"and were re-fetched forever (dispatch attempts so far: %d)", disp.calls)
	}

	got := recipientStatuses(t, db, c.ID)
	require.Zero(t, got[RecipientPending],
		"a permanently-failed recipient must not remain pending, or it is retried forever")
	require.Equal(t, 3, got[RecipientFailed],
		"every recipient whose dispatch failed must reach a terminal failed state")

	require.Equal(t, 3, disp.calls,
		"each recipient must be attempted exactly once, not re-attempted on a later pass")
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

func strPtr(s string) *string { return &s }

type okDispatcher struct{ calls int }

func (d *okDispatcher) Send(context.Context, OutboundEmail) error { d.calls++; return nil }

// The happy path must be untouched: a successful dispatch still reaches
// `sent`, exactly once per recipient.
func TestIntegration_SendWorker_SuccessStillMarksSentOnce(t *testing.T) {
	db := testdb.NewTx(t)
	c := seedCampaignWithRecipients(t, db, 3)
	disp := &okDispatcher{}

	w := NewSendWorker(SendWorkerConfig{
		DB: db, Repo: NewRepository(db), Dispatcher: disp,
		ThemeLoader: noThemeLoader{},
		Logger:      slog.New(slog.NewTextHandler(nopWriter{}, nil)),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, w.dispatchCampaign(ctx, c))

	got := recipientStatuses(t, db, c.ID)
	require.Equal(t, 3, got[RecipientSent])
	require.Zero(t, got[RecipientPending])
	require.Equal(t, 3, disp.calls)
}

// unrecordableRepo sends fine but cannot record the result. Embedding
// Repository means only the one method is overridden.
type unrecordableRepo struct {
	Repository
	attempts int
}

func (r *unrecordableRepo) UpdateRecipientStatus(*gorm.DB, uuid.UUID, string) error {
	r.attempts++
	return errors.New("database unavailable")
}

// If a batch moves nobody out of `pending`, the loop must HALT rather than
// re-send. This is the path where the message was already accepted by the
// provider: retrying would deliver it again, and again, to every recipient in
// the batch. An error is the right answer — it needs a human.
func TestIntegration_SendWorker_NoProgressHaltsInsteadOfResending(t *testing.T) {
	db := testdb.NewTx(t)
	c := seedCampaignWithRecipients(t, db, 3)
	disp := &okDispatcher{}
	repo := &unrecordableRepo{Repository: NewRepository(db)}

	w := NewSendWorker(SendWorkerConfig{
		DB: db, Repo: repo, Dispatcher: disp,
		ThemeLoader: noThemeLoader{},
		Logger:      slog.New(slog.NewTextHandler(nopWriter{}, nil)),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- w.dispatchCampaign(ctx, c) }()

	select {
	case err := <-done:
		require.Error(t, err, "a batch that made no progress must surface as an error")
		require.Contains(t, err.Error(), "no progress")
	case <-ctx.Done():
		t.Fatalf("dispatchCampaign looped instead of halting (dispatch attempts: %d)", disp.calls)
	}

	require.Equal(t, 3, disp.calls,
		"each recipient must be sent to exactly once; halting is what prevents a resend storm")
}
