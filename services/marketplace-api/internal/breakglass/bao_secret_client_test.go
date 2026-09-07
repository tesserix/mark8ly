package breakglass

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
)

func TestBaoSecretPathFor_Format(t *testing.T) {
	path := BaoSecretPathFor("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	require.Equal(t, "kv/mark8ly/marketplace-api/break-glass/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", path)
}

func TestBaoSecretClient_RoundTrip(t *testing.T) {
	fc := carriersecrets.NewFakeClient()
	c := NewBaoSecretClient(fc)
	ctx := context.Background()
	path := BaoSecretPathFor(uuid.NewString())

	require.NoError(t, c.AddVersion(ctx, path, []byte(`{"password":"p","totp_secret":"t"}`)))

	got, err := c.AccessLatest(ctx, path)
	require.NoError(t, err)
	require.Equal(t, `{"password":"p","totp_secret":"t"}`, string(got))
}

// TestBaoSecretClient_AddVersion_PropagatesBackendError proves a Bao
// failure on write surfaces as an error, not a silent success — the
// invariant SecretManager.Upsert depends on to keep the write-before-DB
// ordering safe (see bootstrap.go's Provision).
func TestBaoSecretClient_AddVersion_PropagatesBackendError(t *testing.T) {
	c := NewBaoSecretClient(&failingSecretClient{writeErr: errors.New("bao: unreachable")})

	err := c.AddVersion(context.Background(), BaoSecretPathFor(uuid.NewString()), []byte("payload"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "bao: unreachable")
}

// TestBaoSecretClient_AccessLatest_PropagatesBackendError mirrors the
// write-side check for reads: a backend failure must not resolve to a
// usable (nil-error, empty) blob.
func TestBaoSecretClient_AccessLatest_PropagatesBackendError(t *testing.T) {
	c := NewBaoSecretClient(&failingSecretClient{readErr: errors.New("bao: forbidden")})

	_, err := c.AccessLatest(context.Background(), BaoSecretPathFor(uuid.NewString()))
	require.Error(t, err)
	require.Contains(t, err.Error(), "bao: forbidden")
}

// TestBaoSecretClient_AccessLatest_NotFoundPropagates confirms
// carriersecrets.ErrSecretNotFound survives the adapter via errors.Is,
// rather than being flattened to an opaque string SecretManager.Fetch
// can't distinguish.
func TestBaoSecretClient_AccessLatest_NotFoundPropagates(t *testing.T) {
	fc := carriersecrets.NewFakeClient()
	c := NewBaoSecretClient(fc)

	_, err := c.AccessLatest(context.Background(), BaoSecretPathFor(uuid.NewString()))
	require.Error(t, err)
	require.True(t, errors.Is(err, carriersecrets.ErrSecretNotFound))
}

// failingSecretClient is a minimal carriersecrets.SecretClient stub that
// always errors, for exercising BaoSecretClient's error paths without a
// live OpenBao.
type failingSecretClient struct {
	writeErr error
	readErr  error
}

func (f *failingSecretClient) CreateOrAddVersion(_ context.Context, _ string, _ []byte) error {
	return f.writeErr
}

func (f *failingSecretClient) AccessLatest(_ context.Context, _ string) ([]byte, error) {
	return nil, f.readErr
}

func (f *failingSecretClient) DeleteSecret(_ context.Context, _ string) error {
	return nil
}

var _ carriersecrets.SecretClient = (*failingSecretClient)(nil)
