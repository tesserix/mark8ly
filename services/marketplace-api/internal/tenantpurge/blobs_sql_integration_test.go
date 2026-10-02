//go:build integration

// INTERNAL package: collectBlobRefs is unexported, and the thing worth
// asserting about it needs no fixture — only that every query in
// blobSources is valid against the live schema. A wrong column name there
// fails inside the purge transaction and takes the whole purge with it,
// which a unit test cannot catch and a static column check only
// approximates.
package tenantpurge

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// collectBlobRefs runs inside the purge transaction against every table in
// blobSources. A wrong column name there fails the whole purge, so this
// exercises the real SQL rather than trusting it to be right.
func TestIntegration_CollectBlobRefs_EveryQueryIsValidSQL(t *testing.T) {
	db := testdb.NewDB(t)
	tenantID := uuid.NewString()

	refs, err := collectBlobRefs(context.Background(), db, tenantID)
	require.NoError(t, err, "every query in blobSources must be valid against the live schema")
	require.Empty(t, refs, "an unknown tenant owns no objects")
}
