package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPlatformadminRegisterSitesAgree guards the failure mode in #323: the
// mode.Both engine and the mode.Admin engine each call
// platformadmin.Register with their own Deps literal, and a field added to
// one and not the other means the two deployments differ silently. Three
// distinct failure modes have been observed, including a nil interface
// that panics at runtime.
func TestPlatformadminRegisterSitesAgree(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	require.NoError(t, err)

	var fieldSets [][]string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Register" {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "platformadmin" {
			return true
		}
		for _, arg := range call.Args {
			lit, ok := arg.(*ast.CompositeLit)
			if !ok {
				continue
			}
			var fields []string
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := kv.Key.(*ast.Ident); ok {
					fields = append(fields, key.Name)
				}
			}
			if len(fields) > 0 {
				sort.Strings(fields)
				fieldSets = append(fieldSets, fields)
			}
		}
		return true
	})

	require.Len(t, fieldSets, 2,
		"expected exactly two platformadmin.Register call sites in main.go; "+
			"if a third was added, this test must be updated deliberately")
	require.Equal(t, fieldSets[0], fieldSets[1],
		"the two platformadmin.Register sites construct different Deps field sets — "+
			"one deployment will differ from the other (#323)")
}
