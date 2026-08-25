package trial_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Trial end is derived at exactly one place: trial.EndsAt. Before #353 the
// same arithmetic lived at seven sites, which is why an operator had nothing
// to extend — the expiry cron, Stripe, the merchant's screen and the platform
// console each recomputed it and would have ignored a stored value.
//
// This test fails when an eighth site appears. If you are here because it
// failed: call trial.EndsAt instead of doing the arithmetic, or — if your site
// genuinely is a new definition — change EndsAt and say so.
func TestTrialEndIsDerivedInExactlyOnePlace(t *testing.T) {
	// Matches `<something>.Add(TrialDays * 24 * time.Hour)` and the hardcoded
	// 90-day form that #326 was, in either spacing.
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`Add\(\s*(trial\.)?TrialDays\s*\*\s*24\s*\*\s*time\.Hour\s*\)`),
		regexp.MustCompile(`Add\(\s*90\s*\*\s*24\s*\*\s*time\.Hour\s*\)`),
		regexp.MustCompile(`AddDate\(\s*0\s*,\s*0\s*,\s*-?\s*(trial\.)?TrialDays\s*\)`),
	}

	// The one legitimate site. Paths are relative to the service root.
	allowed := map[string]bool{
		filepath.Join("internal", "billing", "trial", "endsat.go"): true,
	}

	root := filepath.Join("..", "..", "..") // -> services/marketplace-api
	var offenders []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == "vendor" || info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		require.NoError(t, relErr)
		if allowed[rel] {
			return nil
		}
		src, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		for _, re := range patterns {
			if re.Match(src) {
				offenders = append(offenders, rel)
				return nil
			}
		}
		return nil
	})
	require.NoError(t, err)

	require.Empty(t, offenders,
		"these files derive a trial end themselves instead of calling trial.EndsAt — "+
			"that is how #353 happened. Call trial.EndsAt, or change EndsAt if this is genuinely a new definition.")
}
