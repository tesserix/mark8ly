package admin

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strconv"
	"strings"
	"testing"
)

// StoreMiddleware proves that :storeId belongs to the caller's tenant. It
// proves nothing about any OTHER id in the path. A handler keyed on a second
// path id must therefore re-check that the row it loaded actually belongs to
// :storeId, or a uuid from another tenant's store is actionable.
//
// This test finds every admin handler that reads a non-storeId path param and
// requires it to show some store scoping. Handlers that genuinely have none
// are listed in knownUnscoped with a reason — the list is the outstanding
// debt, and it may only shrink.

// knownUnscoped maps "file.go:Handler" to why it is not yet scoped.
// Removing an entry when you fix it is enforced below: a scoped handler that
// is still listed fails the test.
var knownUnscoped = map[string]string{
	// --- Not store-keyed at all: these hang off the tenant, user or device. ---
	"account.go:AccountHandler.RevokeSession":               "session belongs to the user, not a store",
	"push_tokens.go:PushTokenHandler.Delete":                "push token belongs to the device/user, not a store",
	"team.go:TeamHandler.Revoke":                            "invitation is tenant-scoped",
	"apikeys_handler.go:APIKeysHandler.Rotate":              "api key is tenant-scoped",
	"apikeys_handler.go:APIKeysHandler.Revoke":              "api key is tenant-scoped",
	"sso_config.go:SSOConfigHandler.requirePathTenantMatch": "this IS the tenant check, keyed on :tenantId",

	// --- Outstanding cross-tenant debt. Each must compare the loaded row's
	// store_id to :storeId before acting. Tracked in
	// docs/GO-LIVE-PUNCHLIST.md section 1.3. ---
	//
	// Campaigns, csv-imports, reviews and segments were closed by giving
	// each handler a require*InStore helper; the settings three were always
	// scoped through storeFromCtx and only looked unscoped to this test.
}

// scopeMarkers are the ways a handler can demonstrate store scoping.
//
// Note how the matching works before adding one: nodeText flattens a
// handler body to bare identifiers, so a marker has to be a name that
// survives that — a helper or field name. `Param("storeId")` is kept for
// documentation but only ever matches via the `storeId` literal.
var scopeMarkers = []string{
	`Param("storeId")`,
	`requireOrderInStore`,
	`requireReturnInStore`,
	`requireCampaignInStore`,
	`requireSegmentInStore`,
	`requireReviewInStore`,
	`requireJobInStore`,
	`requireMemberInStore`,
	// storeFromCtx returns the store StoreMiddleware resolved from :storeId
	// for this tenant; every caller then filters its query by that store's
	// id, which is the same proof the require*InStore helpers give.
	`storeFromCtx`,
	`MustGet("store")`,
	`Get("store")`,
	"StoreID",
	"storeID",
}

func TestEveryHandlerOnASubResourceScopesToTheStore(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse package: %v", err)
	}

	seen := map[string]bool{}
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			fname := path[strings.LastIndex(path, "/")+1:]
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv == nil || fn.Body == nil || !isGinHandler(fn) {
					continue
				}
				params := pathParams(fn)
				other := false
				for _, p := range params {
					if p != "storeId" {
						other = true
					}
				}
				if !other {
					continue
				}
				key := fname + ":" + recvName(fn) + "." + fn.Name.Name
				body := nodeText(fset, fn)
				scoped := false
				for _, m := range scopeMarkers {
					if strings.Contains(body, m) {
						scoped = true
						break
					}
				}
				reason, listed := knownUnscoped[key]
				seen[key] = true
				switch {
				case scoped && listed:
					t.Errorf("%s is now store-scoped — remove it from knownUnscoped (was: %s)", key, reason)
				case !scoped && !listed:
					t.Errorf("%s reads a non-storeId path param but never scopes to the store. "+
						"Compare the loaded row's store_id against c.Param(\"storeId\") before acting, "+
						"or add it to knownUnscoped with a reason.", key)
				}
			}
		}
	}

	for key := range knownUnscoped {
		if !seen[key] {
			t.Errorf("knownUnscoped lists %s, which no longer exists — remove the entry", key)
		}
	}
}

func recvName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	switch t := fn.Recv.List[0].Type.(type) {
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}

func isGinHandler(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil || len(fn.Type.Params.List) != 1 {
		return false
	}
	star, ok := fn.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Context"
}

func pathParams(fn *ast.FuncDecl) []string {
	var out []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Param" {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if s, err := strconv.Unquote(lit.Value); err == nil {
			out = append(out, s)
		}
		return true
	})
	return out
}

func nodeText(fset *token.FileSet, fn *ast.FuncDecl) string {
	var sb strings.Builder
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.Ident:
			sb.WriteString(v.Name + " ")
		case *ast.BasicLit:
			sb.WriteString(v.Value + " ")
		case *ast.SelectorExpr:
			sb.WriteString(v.Sel.Name + " ")
		}
		return true
	})
	return sb.String()
}
