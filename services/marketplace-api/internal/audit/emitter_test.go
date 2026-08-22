package audit_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

// TestEmit_MissingTenant_LogsDrop guards against the silent-attribution-loss
// defect this series exists to eliminate: a caller that forgets to populate
// Event.TenantID (or set tenant_id on the gin context) must produce a
// visible warning, not a silent no-op. See emitter.go's Emit/buildEntry.
func TestEmit_MissingTenant_LogsDrop(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	em := audit.NewEmitter(audit.EmitterConfig{
		Repo:   audit.NewRepository(),
		Logger: logger,
	})
	t.Cleanup(func() { em.Stop(t.Context()) })

	em.Emit(nil, audit.Event{
		Action:       "tenant.suspend",
		ResourceType: "tenant",
	})

	out := buf.String()
	require.Contains(t, out, "dropping event")
	require.Contains(t, out, "no tenant")
	require.True(t, strings.Contains(out, "tenant.suspend"), "log should identify the action: %s", out)
	require.True(t, strings.Contains(out, "tenant"), "log should identify the resource type: %s", out)
}
