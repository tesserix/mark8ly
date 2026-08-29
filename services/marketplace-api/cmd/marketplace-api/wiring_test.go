package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/tenantgate"
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

// TestTenantGateInvalidatorIsNilInterfaceWhenUnwired covers #341.
//
// tenantGate stays nil when MARKETPLACE_PLATFORM_API_URL is empty. A nil
// *tenantgate.Gate stored in a TenantGateInvalidator interface makes the
// INTERFACE value non-nil, so the guard at tenant_lifecycle.go:244
// (`if h.invalidate != nil`) always fires and dispatches Invalidate on a
// nil receiver. That is safe today only because Invalidate happens to
// check its own receiver — nothing enforces it for the next
// implementation of this interface.
//
// require.Nil is deliberately NOT used: testify unwraps the interface and
// reports a typed-nil pointer as nil, so it would pass against the very
// bug this test exists to catch. The comparison must be `== nil` on the
// interface value itself.
func TestTenantGateInvalidatorIsNilInterfaceWhenUnwired(t *testing.T) {
	got := tenantGateInvalidator(nil)
	require.True(t, got == nil,
		"an unwired gate must produce a nil interface, not a typed-nil *Gate; "+
			"a typed-nil makes every downstream `!= nil` guard dead code")
}

// The non-nil case must still pass the gate through.
func TestTenantGateInvalidatorPassesThroughWiredGate(t *testing.T) {
	g := &tenantgate.Gate{}
	require.Equal(t, tenantGateInvalidator(g), platformadmin.TenantGateInvalidator(g))
}

// TestTenantGateInvalidatorWrappedAtEveryRegisterSite is the regression
// half of #341. The behavioural test above proves the helper normalises a
// nil gate; this proves both platformadmin.Deps literals actually go
// through it, so a future call site cannot quietly reintroduce the
// typed-nil by assigning `tenantGate` directly.
//
// Same narrow scope as TestPlatformadminRegisterSitesAgree: it reads the
// expression each literal assigns to TenantGateInvalidator and requires it
// to be a call, not a bare identifier.
func TestTenantGateInvalidatorWrappedAtEveryRegisterSite(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	require.NoError(t, err)

	var sites int
	ast.Inspect(file, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "TenantGateInvalidator" {
			return true
		}
		sites++
		call, ok := kv.Value.(*ast.CallExpr)
		require.True(t, ok,
			"TenantGateInvalidator at %s must be wrapped by tenantGateInvalidator(), "+
				"not assigned a *tenantgate.Gate directly — a nil one becomes a non-nil interface",
			fset.Position(kv.Pos()))
		fn, ok := call.Fun.(*ast.Ident)
		require.True(t, ok && fn.Name == "tenantGateInvalidator",
			"TenantGateInvalidator at %s must be wrapped by tenantGateInvalidator()",
			fset.Position(kv.Pos()))
		return true
	})

	require.Equal(t, 2, sites,
		"expected the mode.Both and mode.Admin Deps literals; a changed count means "+
			"this test is looking at the wrong thing")
}
