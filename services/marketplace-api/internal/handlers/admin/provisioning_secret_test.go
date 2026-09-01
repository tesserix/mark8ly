package admin

import "testing"

// Regression for a bug shipped in #518: webhook provisioning read
// req.SecretKey, but Stripe's sk_… arrives in req.APIKey — the admin form
// hides the Secret key field for Stripe entirely. The result was that
// provisioning never ran for the only provider it supports, silently.
func TestProvisioningSecretFor(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		req      paymentUpsertRequest
		want     string
	}{
		{
			name:     "stripe reads the API key field, where sk_ actually is",
			provider: "stripe",
			req:      paymentUpsertRequest{APIKey: "sk_test_abc", SecretKey: ""},
			want:     "sk_test_abc",
		},
		{
			name:     "stripe ignores secret_key even when populated",
			provider: "stripe",
			req:      paymentUpsertRequest{APIKey: "sk_test_abc", SecretKey: "ignored"},
			want:     "sk_test_abc",
		},
		{
			name:     "case of the provider name does not matter",
			provider: "Stripe",
			req:      paymentUpsertRequest{APIKey: "sk_test_abc"},
			want:     "sk_test_abc",
		},
		{
			name:     "whitespace-only key counts as absent",
			provider: "stripe",
			req:      paymentUpsertRequest{APIKey: "   "},
			want:     "",
		},
		{
			name:     "an unchanged save carries no key, so nothing is provisioned",
			provider: "stripe",
			req:      paymentUpsertRequest{APIKey: ""},
			want:     "",
		},
		{
			name:     "other providers do not provision webhooks",
			provider: "razorpay",
			req:      paymentUpsertRequest{APIKey: "rzp_test_abc", SecretKey: "shh"},
			want:     "",
		},
		{
			name:     "paypal does not provision webhooks",
			provider: "paypal",
			req:      paymentUpsertRequest{APIKey: "client-id", SecretKey: "client-secret"},
			want:     "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := provisioningSecretFor(tc.provider, tc.req); got != tc.want {
				t.Errorf("provisioningSecretFor(%q) = %q, want %q", tc.provider, got, tc.want)
			}
		})
	}
}
