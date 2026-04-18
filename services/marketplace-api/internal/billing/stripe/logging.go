package stripe

import (
	"encoding/json"
	"errors"
	"fmt"

	sdk "github.com/stripe/stripe-go/v82"
)

// APIError is the sanitized, log-safe representation of a Stripe API error.
// Deliberately omits error.message, request/response bodies, and card data.
type APIError struct {
	HTTPStatus int
	Type       string // "invalid_request_error", "card_error", ...
	Code       string
	RequestID  string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("stripe: %s/%s (status %d request_id=%s)", e.Type, e.Code, e.HTTPStatus, e.RequestID)
}

// ParseAPIError extracts code/type from a Stripe error body without preserving the body.
func ParseAPIError(status int, body, requestID string) error {
	var parsed struct {
		Error struct {
			Code    string `json:"code"`
			Type    string `json:"type"`
			Message string `json:"message"` // intentionally NOT persisted on APIError
		} `json:"error"`
	}
	_ = json.Unmarshal([]byte(body), &parsed)
	return &APIError{
		HTTPStatus: status,
		Type:       parsed.Error.Type,
		Code:       parsed.Error.Code,
		RequestID:  requestID,
	}
}

// toAPIError translates a stripe SDK error into our sanitized *APIError.
// Non-SDK errors are passed through unchanged.
func toAPIError(err error) error {
	if err == nil {
		return nil
	}
	var se *sdk.Error
	if errors.As(err, &se) {
		return &APIError{
			HTTPStatus: se.HTTPStatusCode,
			Type:       string(se.Type),
			Code:       string(se.Code),
			RequestID:  se.RequestID,
		}
	}
	return err
}

// SafeWebhookFields extracts only event.id + event.type for structured logging.
// Never log the raw event payload.
type SafeWebhookFields struct {
	EventID   string `json:"stripe_event_id"`
	EventType string `json:"stripe_event_type"`
}

func ExtractSafeFields(rawEvent []byte) (SafeWebhookFields, error) {
	var e struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(rawEvent, &e); err != nil {
		return SafeWebhookFields{}, err
	}
	return SafeWebhookFields{EventID: e.ID, EventType: e.Type}, nil
}

// SanitizeForLog returns a log-safe string for any error. For APIError it
// returns the sanitized form (no body). For non-API errors it returns
// "internal error" to avoid leaking anything sensitive.
func SanitizeForLog(err error) string {
	if err == nil {
		return ""
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Error()
	}
	return "internal error"
}
