package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestMainCallsNewTenantClaimSetterUnconditionally is the direct
// production-wiring test the reviewer's gap called for: it is not enough
// that newTenantClaimSetter itself guards the typed-nil trap and ignores
// cfg.ZitadelEnabled by construction — main.go could still stop calling
// it, or wrap the call in an `if` that skips it under some condition,
// without any other test noticing.
//
// This parses cmd/server/main.go's actual source (not a copy) and asserts
// TWO things at once:
//  1. main() calls newTenantClaimSetter exactly once.
//  2. That call is a TOP-LEVEL statement in main()'s body — found by
//     scanning main.Body.List directly rather than recursively — so a
//     call moved inside any if/else/switch block (e.g. gated on
//     cfg.ZitadelEnabled) is invisible to this scan and fails the test.
//  3. Its sole argument is the identifier "gipAdmin" — the client kept
//     alive for EnsureTenantClaim — not some other, Zitadel-conditional
//     value.
//
// Mirrors the AST-parsing approach TestMainSetsEveryInternalHandlersField
// (internal_wiring_test.go) already uses for the identical reason: proving
// a fact about main.go's literal wiring that no other test can see.
func TestMainCallsNewTenantClaimSetterUnconditionally(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}

	var mainFunc *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "main" {
			mainFunc = fn
			break
		}
	}
	if mainFunc == nil {
		t.Fatal("main.go must declare func main")
	}

	var calls []*ast.CallExpr
	for _, stmt := range mainFunc.Body.List {
		var call *ast.CallExpr
		switch s := stmt.(type) {
		case *ast.AssignStmt:
			if len(s.Rhs) == 1 {
				if c, ok := s.Rhs[0].(*ast.CallExpr); ok {
					call = c
				}
			}
		case *ast.ExprStmt:
			if c, ok := s.X.(*ast.CallExpr); ok {
				call = c
			}
		}
		if call == nil {
			continue
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "newTenantClaimSetter" {
			calls = append(calls, call)
		}
	}

	if len(calls) != 1 {
		t.Fatalf("found %d top-level call(s) to newTenantClaimSetter in main(), want exactly 1. "+
			"If this is 0, either the call was removed or it was nested inside an if/else/switch "+
			"(e.g. gated on cfg.ZitadelEnabled) — either way EnsureTenantClaim can silently stop "+
			"running against GIP, and marketplace-api's flag-off path reading the tenant_id claim "+
			"breaks for every newly-invited merchant.", len(calls))
	}

	call := calls[0]
	if len(call.Args) != 1 {
		t.Fatalf("newTenantClaimSetter called with %d args, want exactly 1 (gipAdmin)", len(call.Args))
	}
	arg, ok := call.Args[0].(*ast.Ident)
	if !ok || arg.Name != "gipAdmin" {
		t.Fatalf("newTenantClaimSetter's argument = %#v, want the identifier gipAdmin — "+
			"EnsureTenantClaim must always run against the GIP client, never a "+
			"Zitadel-conditional value", call.Args[0])
	}
}

// TestMainNeverReassignsGipAdminAfterConstruction closes a gap the
// previous test leaves open: matching on the argument IDENTIFIER
// "gipAdmin" would still pass if some other statement — e.g. an
// `if cfg.ZitadelEnabled { gipAdmin = nil }` inserted ABOVE the
// newTenantClaimSetter call — reassigned that same variable first. The
// call-site check alone cannot see that; this one can.
//
// So this scans main()'s ENTIRE body (recursively, unlike the top-level-only
// scan above, specifically so it also sees inside if/else/switch blocks)
// for every assignment to an identifier named "gipAdmin", and requires
// there to be EXACTLY ONE: the "gipAdmin = admin" line inside the
// successful-construction branch. A second assignment anywhere —
// including one that only fires when ZitadelEnabled is true — fails this
// test, whatever value it assigns.
//
// This is deliberately narrow: it does not understand control flow or
// prove gipAdmin is non-nil at the newTenantClaimSetter call site (that
// is requireGIPForTenantClaim's job, enforced at runtime, not this
// test's). It only proves the variable is never reassigned after its
// single construction point — cheap, and enough to catch the specific
// shape of regression described above.
func TestMainNeverReassignsGipAdminAfterConstruction(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}

	var mainFunc *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "main" {
			mainFunc = fn
			break
		}
	}
	if mainFunc == nil {
		t.Fatal("main.go must declare func main")
	}

	var assignments []*ast.AssignStmt
	ast.Inspect(mainFunc.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assign.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok && ident.Name == "gipAdmin" {
				assignments = append(assignments, assign)
			}
		}
		return true
	})

	if len(assignments) != 1 {
		t.Fatalf("found %d assignment(s) to gipAdmin in main(), want exactly 1 (its single "+
			"successful-construction assignment). A second assignment anywhere — including "+
			"one only reached when ZitadelEnabled is true — can defeat the "+
			"TestMainCallsNewTenantClaimSetterUnconditionally check above, since that test "+
			"only looks at the call's argument NAME, not the value gipAdmin holds by the "+
			"time the call runs.", len(assignments))
	}
}
