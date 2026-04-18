package stripe_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

func TestSanitizeError_StripsMessageAndBody(t *testing.T) {
	body := `{"error":{"code":"card_declined","type":"card_error","message":"Your card was declined: 4242..."}}`
	err := billingstripe.ParseAPIError(402, body, "req_ABC")

	var apiErr *billingstripe.APIError
	require.True(t, errors.As(err, &apiErr))
	require.Equal(t, "card_declined", apiErr.Code)
	require.Equal(t, "card_error", apiErr.Type)
	require.Equal(t, "req_ABC", apiErr.RequestID)

	msg := apiErr.Error()
	require.NotContains(t, msg, "4242")
	require.NotContains(t, msg, "declined:")
}

func TestExtractSafeFields_IDAndType(t *testing.T) {
	raw := []byte(`{"id":"evt_123","type":"customer.subscription.updated","data":{"object":{"secret":"sk_live_xxx"}}}`)
	f, err := billingstripe.ExtractSafeFields(raw)
	require.NoError(t, err)
	require.Equal(t, "evt_123", f.EventID)
	require.Equal(t, "customer.subscription.updated", f.EventType)
}

func TestParseAPIError_MalformedBodyGivesGenericError(t *testing.T) {
	err := billingstripe.ParseAPIError(500, `<html>nope</html>`, "req_bad")
	var apiErr *billingstripe.APIError
	require.True(t, errors.As(err, &apiErr))
	require.Equal(t, 500, apiErr.HTTPStatus)
	require.Equal(t, "req_bad", apiErr.RequestID)
	// Type/Code may be empty but must not panic.
}
