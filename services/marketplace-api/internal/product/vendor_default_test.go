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

// Replaces TestResolveVendorID_NoLookup_NoOp, which asserted that a nil
// lookup with no VendorID was a silent no-op (#402).
//
// That was the hole. products.vendor_id has been NOT NULL since migration
// 000028 and carries NO foreign key, so the type was the only guard and it
// guarded nothing: a Product could be built with no vendor and the failure
// surfaced as a database error at insert time. It cost 74 integration-test
// failures across three packages before anyone looked at why.
//
// The nil lookup exists for paths that always supply an explicit VendorID —
// and that case still works, because the function returns early before ever
// consulting the lookup. What cannot happen any more is reaching the end
// with nothing set. A path that supplies neither a vendor nor a way to find
// one is a wiring bug, and it should say so here rather than 22 lines later
// in Postgres.
func TestResolveVendorID_NoLookupAndNoVendor_Errors(t *testing.T) {
	in := &CreateRequest{TenantID: "t1"}
	err := resolveVendorID(context.Background(), nil, in)
	require.Error(t, err,
		"a product with no vendor and no way to resolve one must fail here, "+
			"not as a NOT NULL violation at insert time")
	require.True(t, errors.Is(err, apperrors.ErrValidationFailed))
}

// The documented purpose of the nil lookup is preserved: an explicit vendor
// still needs no lookup at all.
func TestResolveVendorID_NoLookupWithExplicitVendor_Succeeds(t *testing.T) {
	explicit := "v-explicit"
	in := &CreateRequest{TenantID: "t1", VendorID: &explicit}
	require.NoError(t, resolveVendorID(context.Background(), nil, in))
	require.Equal(t, "v-explicit", *in.VendorID)
}

// The invariant every caller downstream relies on: once this returns nil,
// VendorID is set and non-empty, so the model can hold a plain string.
func TestResolveVendorID_SuccessGuaranteesANonEmptyVendor(t *testing.T) {
	in := &CreateRequest{TenantID: "t1"}
	require.NoError(t, resolveVendorID(context.Background(), fakeVendorLookup{id: "v-abc"}, in))
	require.NotNil(t, in.VendorID)
	require.NotEmpty(t, *in.VendorID)
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
