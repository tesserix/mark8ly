package onboarding

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// The package doc states that "abandoned" and "expired" are reserved and
// never written. That claim is the whole point of #322: the doc previously
// described a lifecycle the code did not implement, and #283 nearly built an
// abandoned-session counter on `status = 'abandoned'` that would have
// returned zero for every window, forever.
//
// A comment cannot enforce itself. This test does: the moment code starts
// using either constant, it fails and points at the doc that must change with
// it. It is deliberately a source scan rather than a behavioural test —
// "nothing anywhere uses this value" is a property of the package, not of any
// one call.
//
// It walks the AST rather than the file text, and that distinction is
// load-bearing: funnel.go's AbandonedAfter carries a comment explaining that
// `status = 'abandoned'` returns zero forever and why the funnel derives
// abandonment from idle time instead (#283). A text scan flags that comment —
// the very documentation this issue exists to encourage. Identifiers are what
// matter; prose about them is the opposite of a violation.
func TestReservedStatusesAreNeverUsedInCode(t *testing.T) {
	// models.go declares the constants; that is the one legitimate use.
	const declarationFile = "models.go"

	reserved := map[string]struct{}{
		"StatusExpired":   {},
		"StatusAbandoned": {},
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		name := fi.Name()
		return !strings.HasSuffix(name, "_test.go") && name != declarationFile
	}, 0) // no parser.ParseComments — comments are not code.
	if err != nil {
		t.Fatalf("parse package: %v", err)
	}

	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				id, ok := n.(*ast.Ident)
				if !ok {
					return true
				}
				if _, isReserved := reserved[id.Name]; !isReserved {
					return true
				}
				t.Errorf(
					"%s:%d uses %s, but the package doc says that status is reserved and never written.\n"+
						"If a gc now writes it (#322 option 2), update the package doc in %s and this test together — "+
						"and check anything deriving abandonment from last_activity_at (#283) still agrees.",
					path, fset.Position(id.Pos()).Line, id.Name, declarationFile,
				)
				return true
			})
		}
	}
}

// Guards the other half: the constants must keep existing, because the
// migration's CHECK constraint permits them and #283's wire contract talks
// about them. Deleting them without a migration would be the opposite
// mistake.
func TestReservedStatusesStillDeclared(t *testing.T) {
	if StatusExpired != "expired" {
		t.Errorf("StatusExpired = %q, want %q", StatusExpired, "expired")
	}
	if StatusAbandoned != "abandoned" {
		t.Errorf("StatusAbandoned = %q, want %q", StatusAbandoned, "abandoned")
	}
}
