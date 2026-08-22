package audit_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

func TestBuildEntryAttributesOperator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantID := uuid.New()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request, _ = http.NewRequest(http.MethodPost, "/", nil)
	c.Set("tenant_id", tenantID.String())
	c.Set("platform_operator_id", "op_7f3a")
	c.Set("platform_capability", "tenant.suspend")

	entry := audit.BuildEntryForTest(c, audit.Event{
		Action:       "tenant.suspended",
		ResourceType: "tenant",
	})

	require.NotNil(t, entry, "store-less platform event must produce a row")
	require.Equal(t, audit.ActorOperator, entry.ActorType)
	require.NotNil(t, entry.ActorOperatorID)
	require.Equal(t, "op_7f3a", *entry.ActorOperatorID)
	require.NotNil(t, entry.Capability)
	require.Equal(t, "tenant.suspend", *entry.Capability)
	require.Nil(t, entry.StoreID)
}

// Pins ActorType exclusivity: "platform_operator_id" and "user_id" are
// distinct context keys, so an operator claim is classified as
// ActorOperator and never reaches the "user_id" parse path, regardless of
// whether the operator id happens to be a well-formed UUID.
func TestOperatorDoesNotPopulateActorUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request, _ = http.NewRequest(http.MethodPost, "/", nil)
	c.Set("tenant_id", uuid.New().String())
	c.Set("platform_operator_id", uuid.New().String()) // well-formed UUID, but read via a different context key than "user_id"
	c.Set("platform_capability", "audit.read")

	entry := audit.BuildEntryForTest(c, audit.Event{Action: "x", ResourceType: "y"})

	require.NotNil(t, entry)
	require.Nil(t, entry.ActorUserID, "operator id must never land in actor_user_id")
	require.Equal(t, audit.ActorOperator, entry.ActorType)
}
