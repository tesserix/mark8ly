package main

import (
	"encoding/json"
	"testing"

	"github.com/mark8ly/platform-api/pkg/config"
)

func TestOnboardingChatDisabledKeepsEventRetryable(t *testing.T) {
	handler, err := onboardingChatHandler(&config.Config{})
	if err != nil || handler == nil {
		t.Fatalf("disabled feature must still register a retryable handler: %v", err)
	}
	if err := handler(t.Context(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("disabled replica acknowledged an undelivered event")
	}
}

func TestOnboardingChatEnabledRequiresCredentials(t *testing.T) {
	if _, err := onboardingChatHandler(&config.Config{RocketChatEnabled: true}); err == nil {
		t.Fatal("enabled feature accepted incomplete configuration")
	}
}
