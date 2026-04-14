package product

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	apperrors "github.com/mark8ly/marketplace-api/pkg/apperrors"
)

type fakeVendorLookup struct {
	id  string
	err error
}

func (f fakeVendorLookup) GetSelfVendorID(_ context.Context, _ string) (string, error) {
	return f.id, f.err
}

func TestResolveVendorID_SetsWhenMissing(t *testing.T) {
	in := &CreateRequest{TenantID: "t1"}
	require.NoError(t, resolveVendorID(context.Background(), fakeVendorLookup{id: "v-abc"}, in))
	require.NotNil(t, in.VendorID)
	require.Equal(t, "v-abc", *in.VendorID)
}

func TestResolveVendorID_RespectsExplicit(t *testing.T) {
	explicit := "v-explicit"
	in := &CreateRequest{TenantID: "t1", VendorID: &explicit}
	require.NoError(t, resolveVendorID(context.Background(), fakeVendorLookup{id: "v-other"}, in))
	require.Equal(t, "v-explicit", *in.VendorID)
}

func TestResolveVendorID_NoLookup_NoOp(t *testing.T) {
	in := &CreateRequest{TenantID: "t1"}
	require.NoError(t, resolveVendorID(context.Background(), nil, in))
	require.Nil(t, in.VendorID)
}

func TestResolveVendorID_LookupReturnsEmpty_Errors(t *testing.T) {
	in := &CreateRequest{TenantID: "t1"}
	err := resolveVendorID(context.Background(), fakeVendorLookup{id: ""}, in)
	require.Error(t, err)
	require.True(t, errors.Is(err, apperrors.ErrValidationFailed))
}

func TestResolveVendorID_LookupError_Propagates(t *testing.T) {
	in := &CreateRequest{TenantID: "t1"}
	boom := errors.New("db down")
	err := resolveVendorID(context.Background(), fakeVendorLookup{err: boom}, in)
	require.ErrorIs(t, err, boom)
}
