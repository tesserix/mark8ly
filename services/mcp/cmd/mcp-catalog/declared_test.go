package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/mcp/internal/catalog"
	gsmcp "github.com/tesserix/go-shared/mcp"
)

// declaredTools is the registry record's tool list, copied by hand from
// devai/architecture/registry-seeds/mcp-servers/mark8ly-mcp-catalog.yaml.
// #412 was not a typo — it was that nothing compared what was DECLARED
// against what was SERVED. This is that comparison.
var declaredTools = []string{
	"get_store_branding",
	"get_store_product",
	"list_products_by_category",
	"list_store_categories",
	"list_store_products",
}

func TestServedToolsMatchTheRegistryRecord(t *testing.T) {
	r := gsmcp.NewRegistry()
	require.NoError(t, catalog.RegisterTools(r, &catalog.Client{}))

	assert.Equal(t, declaredTools, r.Names(),
		"the registry seed and the server disagree — update the seed in devai "+
			"and this list together, or an agent is offered a tool that does not "+
			"exist (or denied one that does)")
}
