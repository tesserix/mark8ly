package breakglass

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSecretManager_RoundTrip(t *testing.T) {
	sm := NewSecretManager(NewFakeSecretClient())
	ctx := context.Background()

	blob := Blob{Password: "hunter2!Ab", TOTPSecret: "BASE32XYZ", GeneratedAt: time.Now().UTC()}
	require.NoError(t, sm.Upsert(ctx, "/path", blob))

	got, err := sm.Fetch(ctx, "/path")
	require.NoError(t, err)
	require.Equal(t, blob.Password, got.Password)
	require.Equal(t, blob.TOTPSecret, got.TOTPSecret)
}

func TestSecretManager_Upsert_StampsGeneratedAtIfMissing(t *testing.T) {
	sm := NewSecretManager(NewFakeSecretClient())
	ctx := context.Background()

	require.NoError(t, sm.Upsert(ctx, "/stamp", Blob{Password: "p!A9b", TOTPSecret: "T"}))
	got, err := sm.Fetch(ctx, "/stamp")
	require.NoError(t, err)
	require.False(t, got.GeneratedAt.IsZero(), "expected GeneratedAt to be auto-stamped")
}

func TestSecretManager_Fetch_MissingPath(t *testing.T) {
	sm := NewSecretManager(NewFakeSecretClient())
	_, err := sm.Fetch(context.Background(), "/nope")
	require.Error(t, err)
}

func TestSecretManager_Fetch_RejectsBlobMissingFields(t *testing.T) {
	fc := NewFakeSecretClient()
	// Manually inject a payload missing totp_secret.
	require.NoError(t, fc.AddVersion(context.Background(), "/bad", []byte(`{"password":"x","generated_at":"2026-04-18T00:00:00Z"}`)))
	sm := NewSecretManager(fc)

	_, err := sm.Fetch(context.Background(), "/bad")
	require.Error(t, err)
}

func TestSecretPathFor_Format(t *testing.T) {
	path := SecretPathFor("tesserix-prod", "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	require.Equal(t, "/projects/tesserix-prod/secrets/break-glass-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", path)
}
