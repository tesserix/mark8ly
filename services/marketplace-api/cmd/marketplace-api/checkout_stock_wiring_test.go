package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

// #230: stock enforcement lives behind a builder call, and a builder call
// that is simply absent leaves the storefront overselling exactly as it did
// before — silently, with every test still green.
//
// That is the same shape as #341: a guard that is not wired is
// indistinguishable from no guard. Both checkout handlers must be wired, and
// the EXT one especially: routes.go prefers it whenever it is set, so
// enforcement on the simple handler alone would fix nothing in production.
//
// Narrow by design, exactly like TestPlatformadminRegisterSitesAgree: it
// reads method names off the builder chains and nothing else.
func TestMainWiresStockHoldsIntoCheckout(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	require.NoError(t, err)

	// constructor name -> whether WithStockHolds appears in its chain
	wired := map[string]bool{
		"NewCheckoutHandler":    false,
		"NewCheckoutExtHandler": false,
	}

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		root, methods := unwindChain(call)
		if root == "" {
			return true
		}
		if _, tracked := wired[root]; !tracked {
			return true
		}
		for _, m := range methods {
			if m == "WithStockHolds" {
				wired[root] = true
			}
		}
		return true
	})

	for ctor, ok := range wired {
		require.True(t, ok,
			"%s must be built .WithStockHolds(...) — without it a storefront sale "+
				"does not touch inventory and oversells without limit (#230)", ctor)
	}
}

// unwindChain walks a builder chain back to its constructor, returning the
// constructor's function name and every method called on it.
func unwindChain(call *ast.CallExpr) (string, []string) {
	var methods []string
	for {
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			// Reached the root call: storefront.NewCheckoutHandler(...) or
			// a bare NewCheckoutHandler(...).
			switch fn := call.Fun.(type) {
			case *ast.Ident:
				return fn.Name, methods
			}
			return "", methods
		}
		inner, ok := sel.X.(*ast.CallExpr)
		if !ok {
			// sel.X is a package identifier, so sel.Sel is the constructor.
			return sel.Sel.Name, methods
		}
		methods = append(methods, sel.Sel.Name)
		call = inner
	}
}
