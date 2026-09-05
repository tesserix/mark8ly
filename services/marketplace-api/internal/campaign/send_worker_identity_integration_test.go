//go:build integration

package campaign

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// capturingDispatcher records the OutboundEmail the worker built.
type capturingDispatcher struct {
	mu   sync.Mutex
	msgs []OutboundEmail
}

func (d *capturingDispatcher) Send(_ context.Context, msg OutboundEmail) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.msgs = append(d.msgs, msg)
	return nil
}

// storeThemeStub supplies the store identity the loader reads from the
// database in production.
type storeThemeStub struct{}

func (storeThemeStub) LoadTheme(context.Context, uuid.UUID) (CampaignTheme, error) {
	return CampaignTheme{
		StoreName:         "Nadia's Ceramics",
		StoreSlug:         "nadias-ceramics",
		StoreContactEmail: "hello@nadiasceramics.com",
	}, nil
}

// TestIntegration_SendWorker_CarriesStoreSenderIdentity pins the hop the
// dispatcher-level test in internal/email cannot see (#718).
//
// That test hands EmailDispatcher a Sender it built itself, so it proves
// the dispatcher APPLIES an identity — not that the worker SUPPLIES one.
// Emptying the worker's Sender literal left every other test in this
// service green, which is precisely the gap this closes: the assertion
// is on what the worker produces from a loaded theme, not on a value the
// test handed it.
func TestIntegration_SendWorker_CarriesStoreSenderIdentity(t *testing.T) {
	db := testdb.NewTx(t)
	c := seedCampaignWithRecipients(t, db, 2)
	disp := &capturingDispatcher{}

	w := NewSendWorker(SendWorkerConfig{
		DB: db, Repo: NewRepository(db), Dispatcher: disp,
		ThemeLoader: storeThemeStub{},
		Logger:      slog.New(slog.NewTextHandler(nopWriter{}, nil)),
	})

	require.NoError(t, w.dispatchCampaign(context.Background(), c))
	require.Len(t, disp.msgs, 2)

	for _, msg := range disp.msgs {
		require.Equal(t, "Nadia's Ceramics", msg.Sender.Name,
			"the worker must carry the loaded store name into the envelope")
		require.Equal(t, "nadias-ceramics", msg.Sender.Slug)
		require.Equal(t, "hello@nadiasceramics.com", msg.Sender.ContactEmail)
	}
}
