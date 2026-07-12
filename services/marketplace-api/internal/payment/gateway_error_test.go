package payment

import (
	"errors"
	"fmt"
	"testing"
)

func TestGatewayError_Permanent(t *testing.T) {
	cases := []struct {
		status int
		want   bool
	}{
		{400, true},  // bad request — request itself is wrong
		{402, true},  // card declined / already refunded
		{404, true},  // unknown payment id
		{422, true},  // unprocessable (PayPal refund-exceeded)
		{408, false}, // request timeout — transient
		{429, false}, // rate limited — transient
		{500, false}, // provider outage — transient
		{502, false},
		{503, false},
	}
	for _, tc := range cases {
		ge := &GatewayError{Provider: "stripe", StatusCode: tc.status, Body: "x"}
		if got := ge.Permanent(); got != tc.want {
			t.Fatalf("status %d: Permanent() = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestIsPermanentGatewayError_UnwrapsAndDefaults(t *testing.T) {
	// Wrapped GatewayError is still recognised through the %w chain.
	wrapped := fmt.Errorf("payment service: gateway refund: %w",
		&GatewayError{Provider: "stripe", StatusCode: 400, Body: "bad"})
	if !IsPermanentGatewayError(wrapped) {
		t.Fatal("wrapped permanent GatewayError not detected")
	}

	// A transient GatewayError is not permanent.
	if IsPermanentGatewayError(&GatewayError{StatusCode: 503}) {
		t.Fatal("503 classified permanent")
	}

	// Non-GatewayError (network blip, context cancel) defaults to transient.
	if IsPermanentGatewayError(errors.New("dial tcp: connection refused")) {
		t.Fatal("plain error classified permanent (should default transient)")
	}
	if IsPermanentGatewayError(nil) {
		t.Fatal("nil classified permanent")
	}
}
