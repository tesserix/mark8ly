package main

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/mark8ly/platform-api/internal/onboarding"
	"github.com/mark8ly/platform-api/internal/outbox"
	"github.com/mark8ly/platform-api/pkg/config"
)

func onboardingChatHandler(cfg *config.Config) (outbox.Handler, error) {
	if !cfg.RocketChatEnabled {
		// Disabled replicas can claim new events briefly during a rolling enablement.
		return func(context.Context, json.RawMessage) error {
			return errors.New("rocketchat: delivery is not enabled on this replica")
		}, nil
	}
	return onboarding.NewRocketChatHandler(onboarding.RocketChatConfig{
		URL: cfg.RocketChatURL, UserID: cfg.RocketChatUserID,
		Token: cfg.RocketChatToken, RoomID: cfg.RocketChatRoomID,
	})
}
