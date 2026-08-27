package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPlatformadminRegisterSitesAgree guards one narrow slice of the
// failure mode in #323: the mode.Both engine and the mode.Admin engine each
// call platformadmin.Register with their own Deps literal, and a field
// added to one and not the other means the two deployments differ silently.
//
// It compares the SET OF FIELD NAMES each literal sets, and nothing more.
// Two sites that both set `DB` still pass this test even if one passes a
// real connection and the other passes nil — values, types, and ordering
// are out of scope.
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

// TestBillingTemplatesRegisteredAfterSeeding guards spec §4: "No seed
// migration ... A key with no row simply renders from its embedded default."
//
// SeedFromEmbedded inserts every REGISTERED fallback as status='published',
// and Loader.Render prefers a published row. If email.RegisterFallbacks ran
// before it, the 11 billing templates would be seeded as published rows —
// day one identical, and then permanently stale, because the seed is
// ON CONFLICT (key) DO NOTHING: an edit to templates_content.go would
// deploy and silently never reach a merchant.
//
// Ordering in a single function body is exactly the kind of thing a later
// refactor reshuffles without noticing, so it is asserted here on source
// position rather than left to review.
func TestBillingTemplatesRegisteredAfterSeeding(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	require.NoError(t, err)

	var registerPos, seedPos token.Pos
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch sel.Sel.Name {
		case "RegisterFallbacks":
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "email" {
				registerPos = call.Pos()
			}
		case "SeedFromEmbedded":
			seedPos = call.Pos()
		}
		return true
	})

	require.NotZero(t, registerPos, "email.RegisterFallbacks call not found in main.go")
	require.NotZero(t, seedPos, "SeedFromEmbedded call not found in main.go")
	require.Greater(t, int(registerPos), int(seedPos),
		"email.RegisterFallbacks must run AFTER SeedFromEmbedded, or the billing "+
			"templates get seeded as published rows and go stale forever (spec §4)")
}

// TestInboxIsWiredAtBothRegisterSites is the specific guard for #280.
//
// TestPlatformadminRegisterSitesAgree above only proves the two Deps literals
// AGREE; two literals that both omit Inbox agree perfectly and leave
// GET /admin/inbox answering 404, which is exactly the state this test was
// written to end. Naming the field explicitly is what makes the route's
// absence a test failure rather than a silent 404.
func TestInboxIsWiredAtBothRegisterSites(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	require.NoError(t, err)

	sites := 0
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
			sites++
			var names []string
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := kv.Key.(*ast.Ident); ok {
					names = append(names, key.Name)
				}
			}
			require.Containsf(t, names, "Inbox",
				"a platformadmin.Deps literal does not set Inbox; GET /admin/inbox 404s (#280)")
		}
		return true
	})

	require.Equal(t, 2, sites, "expected exactly two platformadmin.Register sites")
}
