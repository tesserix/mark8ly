package giftcard

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// TestService_SetStatus_RejectsUnsupportedTarget verifies that the service
// layer rejects unsupported target statuses (anything but active/disabled)
// BEFORE opening a transaction. The test uses a stubRepo that panics on any
// unstubbed call — if validation doesn't catch the unsupported status and
// prevent the repo call, the test will panic loudly rather than silently pass.
func TestService_SetStatus_RejectsUnsupportedTarget(t *testing.T) {
	svc := NewService(nil, &stubRepo{}, nil)

	storeID := uuid.New()
	tenantID := uuid.New()
	id := uuid.New()

	// Try to set status to StatusPending (unsupported target).
	_, err := svc.SetStatus(context.Background(), storeID, tenantID, id, StatusPending)

	// Should return a validation error.
	require.Error(t, err)
	var ae *apperrors.Error
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, apperrors.CodeValidationFailed, ae.Code)
}
