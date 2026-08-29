package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/platform-api/internal/routes"
)

// routes.MountInternal owns the route-to-guard mapping and is tested
// directly (#323). What that test cannot see is this file: main.go could
// leave a field of InternalHandlers unset, which would silently unmount
// the route rather than move it.
//
// So this asserts the literal in main.go populates every declared field.
// Narrow on purpose — it reads field NAMES only, exactly like
// marketplace-api's TestPlatformadminRegisterSitesAgree. Two fields given
// each other's values would still pass; a field quietly dropped would
// not, and that is the failure that leaves no trace anywhere else.
func TestMainSetsEveryInternalHandlersField(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	require.NoError(t, err)

	var got []string
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		sel, ok := lit.Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "InternalHandlers" {
			return true
		}
		for _, elt := range lit.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				if key, ok := kv.Key.(*ast.Ident); ok {
					got = append(got, key.Name)
				}
			}
		}
		return true
	})

	var want []string
	rt := reflect.TypeOf(routes.InternalHandlers{})
	for i := 0; i < rt.NumField(); i++ {
		want = append(want, rt.Field(i).Name)
	}

	require.ElementsMatch(t, want, got,
		"every field of routes.InternalHandlers must be set in main.go — an unset "+
			"field is a route that silently does not exist in the running service")
}
