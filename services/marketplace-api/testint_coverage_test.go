package marketplaceapi

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// testIntExclusions are integration packages deliberately left out of the
// `test-int` target. Each value is the reason, and the test below fails on an
// empty one — an exclusion without a justification is how the gap this guard
// exists to close reopens.
//
// Empty is the correct state. Add an entry only when a package genuinely
// cannot run in the target, not when it is merely failing: a failing package
// is a bug to fix, and hiding it here is precisely the mistake of #446.
var testIntExclusions = map[string]string{}

// TestEveryIntegrationPackageIsInTheTestIntTarget fails when a package
// containing `//go:build integration` files is not run by `make test-int`.
//
// This is deliberately a UNIT test — no build tag, no database. A guard that
// only runs under the conditions it is guarding would share the fate of what
// it guards. `go test ./...` runs it.
//
// WHY THIS EXISTS. The package list in the Makefile is hand-maintained, so a
// package is only ever covered by someone remembering to add it. When
// milestone "Correctness & data integrity" was worked, 32 of the service's 60
// integration packages were absent from the target — more than half — and
// four separate defects had survived in them because the test that caught
// each one had never run:
//
//   - #397 asserted a blocked-downgrade audit row persisted; it was rolled
//     back, and the assertion had been failing for as long as the bug existed.
//   - #399 asserted the trial ramp was idempotent; re-runs were refunding
//     consumed budget.
//   - #398's appeal test failed with SQLSTATE 22001 on every realistic appeal.
//   - #446's own two failures, found only by measuring the gap.
//
// In each case the test was correct and had simply never been executed. The
// cost of the gap is not a missing test — it is a *correct* test whose failure
// nobody sees. This guard makes that impossible to reintroduce silently.
//
// It is the same shape as TestExpectedSchemaVersionMatchesHighestMigration in
// this package (adding a migration without bumping the version), and as
// tenantpurge's and customererasure's schema-coverage guards (a table escaping
// a plan). All four fail loudly at the moment the omission is made rather than
// at the moment it costs something.
func TestEveryIntegrationPackageIsInTheTestIntTarget(t *testing.T) {
	pkgs, err := integrationPackages(".")
	if err != nil {
		t.Fatalf("scan for integration packages: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatal("found no integration packages at all — the scan is broken, " +
			"and a guard that finds nothing passes for the wrong reason")
	}

	patterns, err := testIntPatterns(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatalf("parse test-int target: %v", err)
	}
	if len(patterns) == 0 {
		t.Fatal("parsed no package patterns from the test-int target — the " +
			"parser is broken, or the target moved")
	}

	var uncovered []string
	for _, pkg := range pkgs {
		if _, excluded := testIntExclusions[pkg]; excluded {
			continue
		}
		if !coveredByAny(pkg, patterns) {
			uncovered = append(uncovered, pkg)
		}
	}
	sort.Strings(uncovered)

	if len(uncovered) > 0 {
		t.Errorf("these packages have integration tests that `make test-int` "+
			"never runs, so a failure in them is invisible:\n  %s\n\n"+
			"Add each to the test-int package list in the root Makefile. If one "+
			"genuinely cannot run there, add it to testIntExclusions WITH a "+
			"reason — but a package that merely FAILS is a bug to fix, not to "+
			"exclude (#446).", strings.Join(uncovered, "\n  "))
	}

	for pkg, reason := range testIntExclusions {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("exclusion %q has no justification — leaving a package out "+
				"of the integration runner is a decision that needs a stated reason", pkg)
		}
		if coveredByAny(pkg, patterns) {
			t.Errorf("%q is both excluded and covered by the test-int list; "+
				"remove the stale exclusion so the map cannot accumulate dead weight", pkg)
		}
	}
}

// integrationPackages returns every directory under root holding at least one
// file with a `//go:build integration` constraint, as slash-separated paths
// relative to root.
func integrationPackages(root string) ([]string, error) {
	seen := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip hidden trees (.claude holds worktrees, whose packages would
			// otherwise be counted twice) and node_modules.
			//
			// `vendor` is skipped only at the module ROOT, where Go gives it
			// its special meaning. This service has a real `internal/vendor`
			// package — skipping it by name anywhere would have hidden it from
			// this guard, which is the exact failure the guard exists to catch.
			name := d.Name()
			rel := filepath.ToSlash(path)
			if name != "." && (strings.HasPrefix(name, ".") || name == "node_modules" || rel == "vendor" || rel == "./vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		tagged, err := hasIntegrationTag(path)
		if err != nil || !tagged {
			return err
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		seen[strings.TrimPrefix(dir, "./")] = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Strings(out)
	return out, nil
}

// hasIntegrationTag reports whether the file carries a `//go:build integration`
// constraint. Only the build-constraint prologue is read: a `//go:build` line
// must precede the package clause, so scanning past it would only risk
// matching the string in unrelated code.
func hasIntegrationTag(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "package ") {
			return false, nil
		}
		if strings.HasPrefix(line, "//go:build") && strings.Contains(line, "integration") {
			return true, nil
		}
	}
	return false, sc.Err()
}

// testIntPatterns extracts the `./...`-style package arguments from the
// marketplace-api section of the Makefile's test-int target.
//
// The section is identified by its `cd services/marketplace-api` line and runs
// to the end of that shell continuation, so the platform-api and auth-bff
// sections of the same target are not mistaken for it.
func testIntPatterns(makefilePath string) ([]string, error) {
	raw, err := os.ReadFile(makefilePath)
	if err != nil {
		return nil, err
	}

	var patterns []string
	inSection := false
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "cd services/marketplace-api") {
			inSection = true
			continue
		}
		if !inSection {
			continue
		}
		for _, field := range strings.Fields(strings.TrimSuffix(trimmed, "\\")) {
			if strings.HasPrefix(field, "./") {
				patterns = append(patterns, strings.TrimPrefix(field, "./"))
			}
		}
		// The section ends at the first line that does not continue.
		if !strings.HasSuffix(trimmed, "\\") {
			inSection = false
		}
	}
	return patterns, nil
}

// coveredByAny reports whether pkg is matched by a `go test` package pattern,
// honouring the `/...` recursive suffix.
func coveredByAny(pkg string, patterns []string) bool {
	for _, p := range patterns {
		if base, ok := strings.CutSuffix(p, "/..."); ok {
			if pkg == base || strings.HasPrefix(pkg, base+"/") {
				return true
			}
			continue
		}
		if pkg == p {
			return true
		}
	}
	return false
}
