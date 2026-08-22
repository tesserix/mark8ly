//go:build integration

package webhookevents_test

import (
	"encoding/json"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/webhookevents"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
	"github.com/stretchr/testify/require"
)

func TestStripeWebhookEvent_InsertOnConflictNoop(t *testing.T) {
	db := testdb.NewDB(t, "stripe_webhook_events")

	payload, _ := json.Marshal(map[string]any{"foo": "bar"})
	evt := webhookevents.StripeWebhookEvent{
		EventID:   "evt_idempotent",
		EventType: "customer.subscription.updated",
		Payload:   payload,
	}
	require.NoError(t, db.Create(&evt).Error)

	err := db.Exec(`INSERT INTO stripe_webhook_events (event_id, event_type, payload)
                    VALUES (?, ?, ?::jsonb) ON CONFLICT (event_id) DO NOTHING`,
		evt.EventID, evt.EventType, string(payload)).Error
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.Table("stripe_webhook_events").Where("event_id=?", evt.EventID).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
