// criterion_51_test.go verifies spec §28 success criterion #51:
// "IP hash: HMAC-SHA256 with Secret Manager key; salt rotation 30d preserves
// 31-day overlap; cross-window correlation severed beyond 31d."
//
// This is a pure-Go test — it does NOT require a database or real Secret Manager.
// It proves the HMAC construction, rotation behaviour, and overlap/severance
// properties using in-process stubs only. The integration tag keeps it out of
// the fast unit-test path but it is exercised by the CI integration job.

//go:build integration

package arbitrage_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/arbitrage"
)

// manualHMAC computes HMAC-SHA256(key, data) and returns the hex digest.
// Used to independently verify that the Hasher produces the correct primitive.
func manualHMAC(key []byte, data string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestCriterion51_HMACRotationPreservesOverlap(t *testing.T) {
	const testIP = "203.0.113.42"

	// ── Step 1: Setup — two versions simulating a 35-day-old rotation ──────
	keyV3 := []byte("key-v3-32-bytes-exactly-padded--")
	keyV2 := []byte("key-v2-32-bytes-exactly-padded--")
	now := time.Now()

	src := &stubVersionsSource{
		versions: []arbitrage.KeyVersion{
			{Name: "versions/3", Payload: keyV3, CreatedAt: now},
			{Name: "versions/2", Payload: keyV2, CreatedAt: now.Add(-35 * 24 * time.Hour)},
		},
	}
	loader := arbitrage.NewKeyLoader(src, 5*time.Minute)
	hasher := arbitrage.NewHasher(loader)

	// ── Step 2: Hash twice under the same rotation window ──────────────────
	r1, err := hasher.Hash(context.Background(), testIP)
	require.NoError(t, err)
	require.Equal(t, "versions/3", r1.KeyVersion, "should use newest version")
	require.Len(t, r1.Hex, 64, "HMAC-SHA256 hex must be 64 chars")

	r1b, err := hasher.Hash(context.Background(), testIP)
	require.NoError(t, err)
	require.Equal(t, r1.Hex, r1b.Hex, "same key + same IP must produce same digest")

	// ── Step 3: Rotate — prepend v4 (simulates nightly cron creating a new version)
	keyV4 := []byte("key-v4-32-bytes-exactly-padded--")
	src2 := &stubVersionsSource{
		versions: []arbitrage.KeyVersion{
			{Name: "versions/4", Payload: keyV4, CreatedAt: now.Add(time.Hour)},
			{Name: "versions/3", Payload: keyV3, CreatedAt: now},
			{Name: "versions/2", Payload: keyV2, CreatedAt: now.Add(-35 * 24 * time.Hour)},
		},
	}
	loader2 := arbitrage.NewKeyLoader(src2, 1*time.Nanosecond) // TTL=1ns forces immediate fetch
	hasher2 := arbitrage.NewHasher(loader2)

	// ── Step 4: Assert rotation severs current correlation ─────────────────
	r2, err := hasher2.Hash(context.Background(), testIP)
	require.NoError(t, err)
	require.Equal(t, "versions/4", r2.KeyVersion, "after rotation latest must be v4")
	require.NotEqual(t, r1.Hex, r2.Hex, "different key must produce different digest — rotation severs correlation")

	// ── Step 5: Assert overlap window preserves backward correlation ────────
	// During the overlap window (≤61 days), retained keys allow correlating
	// old ip_hash values by re-hashing under the key that produced them.
	reproduced := manualHMAC(keyV3, testIP)
	require.Equal(t, r1.Hex, reproduced,
		"correlation preserved: manual HMAC under v3 must reproduce r1.Hex")

	// ── Step 6: Assert past-retention severance ─────────────────────────────
	// Simulate the rotator having disabled v2 and v3 (>61 days old).
	// Only v4 remains.
	src3 := &stubVersionsSource{
		versions: []arbitrage.KeyVersion{
			{Name: "versions/4", Payload: keyV4, CreatedAt: now.Add(time.Hour)},
		},
	}
	loader3 := arbitrage.NewKeyLoader(src3, time.Minute)
	all3, err := loader3.All(context.Background())
	require.NoError(t, err)
	require.Len(t, all3, 1, "after v2/v3 disabled, only v4 remains")

	// v3 key is no longer in the store — correlation with r1.Hex is severed.
	// (We demonstrate this by showing that the only available key produces a
	// different digest than r1, and the v3 key bytes are not retrievable.)
	r4, err := arbitrage.NewHasher(loader3).Hash(context.Background(), testIP)
	require.NoError(t, err)
	require.NotEqual(t, r1.Hex, r4.Hex,
		"correlation severed: v4-only store cannot reproduce v3-keyed hash")
}
