// Package pushevents publishes merchant device-push events to Pub/Sub. A push
// subscription delivers them to the OIDC-gated /pubsub/merchant-push endpoint,
// which fans them out to a store's admin devices via the Expo Push API. This
// decouples "a merchant notification happened" from "send a device push", so
// the producer (notification.Service) never blocks on or fails because of push
// delivery, and delivery can later move to a dedicated worker without touching
// producers.
package pushevents

import (
	"context"
	"encoding/json"
	"log/slog"

	"cloud.google.com/go/pubsub"
	"github.com/google/uuid"
)

// Event is the merchant-push message shape. Kept flat and self-contained so
// the delivery handler needs no cross-service lookup: store_id targets the
// devices, the rest is the notification content.
type Event struct {
	Type     string `json:"type"`
	StoreID  string `json:"store_id"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	DeepLink string `json:"deep_link,omitempty"`
}

// Publisher publishes merchant-push events to a Pub/Sub topic.
type Publisher struct {
	client *pubsub.Client
	topic  *pubsub.Topic
	logger *slog.Logger
}

// NewPublisher connects to Pub/Sub and returns a Publisher for topicName.
func NewPublisher(ctx context.Context, projectID, topicName string, logger *slog.Logger) (*Publisher, error) {
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return &Publisher{client: client, topic: client.Topic(topicName), logger: logger}, nil
}

// PublishPush satisfies notification.PushPublisher. Fire-and-forget: the
// Pub/Sub client batches and sends asynchronously, and a delivery error is
// logged, never returned — a failed push must not fail the write that
// triggered it. The publish uses a cancel-free context so it survives the
// request that spawned it.
func (p *Publisher) PublishPush(ctx context.Context, storeID uuid.UUID, notifType, title, body, deepLink string) {
	if p == nil || p.topic == nil {
		return
	}
	data, err := json.Marshal(Event{
		Type:     notifType,
		StoreID:  storeID.String(),
		Title:    title,
		Body:     body,
		DeepLink: deepLink,
	})
	if err != nil {
		if p.logger != nil {
			p.logger.Error("push event marshal failed", "err", err)
		}
		return
	}
	res := p.topic.Publish(context.WithoutCancel(ctx), &pubsub.Message{Data: data})
	go func() {
		if _, err := res.Get(context.Background()); err != nil && p.logger != nil {
			p.logger.Error("push event publish failed",
				"store_id", storeID.String(), "type", notifType, "err", err)
		}
	}()
}

// Close flushes and releases the Pub/Sub client.
func (p *Publisher) Close() error {
	if p == nil || p.topic == nil {
		return nil
	}
	p.topic.Stop()
	return p.client.Close()
}
