package email_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/email"
)

func TestNoOpClient_Send_ReturnsNil(t *testing.T) {
	client := email.NoOpClient{Logger: slog.Default()}
	err := client.Send(context.Background(), email.TemplateDunningDay5, "merchant@example.com", map[string]any{
		"store_id": "abc123",
	})
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestNoOpClient_Send_NilLogger_ReturnsNil(t *testing.T) {
	client := email.NoOpClient{Logger: nil}
	err := client.Send(context.Background(), email.TemplateDunningDay7, "merchant@example.com", nil)
	if err != nil {
		t.Fatalf("expected nil error with nil logger, got: %v", err)
	}
}

func TestNoOpClient_ImplementsClient(t *testing.T) {
	var _ email.Client = email.NoOpClient{}
}
