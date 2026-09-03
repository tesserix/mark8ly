package zitadellogin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sourceFiles(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		out[name] = string(b)
	}
	return out
}

func TestFinalizeIsOnlyCalledFromSufficiency(t *testing.T) {
	for name, src := range sourceFiles(t) {
		if name == "sufficiency.go" {
			continue
		}
		if strings.Contains(src, "c.finalize(") || strings.Contains(src, ".finalize(ctx") {
			t.Errorf("%s calls finalize; the OIDC finalize call must stay behind a sufficiency decision, "+
				"because Zitadel does not enforce forceMfa for a login client", name)
		}
	}
}

func TestSufficientWitnessIsOnlyConstructedInSufficiency(t *testing.T) {
	for name, src := range sourceFiles(t) {
		if name == "sufficiency.go" {
			continue
		}
		if strings.Contains(src, "sufficient{") {
			t.Errorf("%s constructs the sufficient witness; only sufficiency.go may", name)
		}
	}
}

func TestSufficiencyNeverUsesTheUnscopedDisplayPolicy(t *testing.T) {
	src := sourceFiles(t)["sufficiency.go"]
	if strings.Contains(src, "InstanceLoginPolicyForDisplay") {
		t.Error("sufficiency.go references InstanceLoginPolicyForDisplay; enforcement must read the " +
			"user's own org policy via LoginPolicyForOrg, or a user in one org is judged by another org's policy")
	}
}
