package stripe_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

func TestVerifySignature_AcceptsValidSignature(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{"id":"evt_1","type":"customer.subscription.updated"}`)
	now := time.Unix(1_712_000_000, 0)

	sig := billingstripe.BuildSignatureForTesting(payload, secret, now)
	eventID, eventType, err := billingstripe.VerifySignature(payload, sig, secret, now)
	require.NoError(t, err)
	require.Equal(t, "evt_1", eventID)
	require.Equal(t, "customer.subscription.updated", eventType)
}

func TestVerifySignature_RejectsOldTimestamp(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{"id":"evt_1","type":"x"}`)
	nowSigning := time.Unix(1_712_000_000, 0)
	nowVerifying := nowSigning.Add(6 * time.Minute)

	sig := billingstripe.BuildSignatureForTesting(payload, secret, nowSigning)
	_, _, err := billingstripe.VerifySignature(payload, sig, secret, nowVerifying)
	require.True(t, errors.Is(err, billingstripe.ErrStaleSignature))
}

func TestVerifySignature_RejectsTamperedPayload(t *testing.T) {
	secret := "whsec_test"
	payload := []byte(`{"id":"evt_1","type":"x"}`)
	now := time.Unix(1_712_000_000, 0)
	sig := billingstripe.BuildSignatureForTesting(payload, secret, now)

	_, _, err := billingstripe.VerifySignature([]byte(`{"id":"evt_2","type":"x"}`), sig, secret, now)
	require.True(t, errors.Is(err, billingstripe.ErrBadSignature))
}

func TestVerifySignature_RejectsMalformedHeader(t *testing.T) {
	_, _, err := billingstripe.VerifySignature([]byte(`{}`), "garbage", "secret", time.Now())
	require.True(t, errors.Is(err, billingstripe.ErrMalformedHeader))
}
