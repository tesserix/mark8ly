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
// genuinely is a new definition — change EndsAt and say so, or — if the
// duration is not a trial end at all — add the file to `allowed` with a comment
// explaining why.
//
// NOTE: This regex tripwire is a heuristic, not a proof. It does not match:
//   - negated forms (Add(-90 * 24 * time.Hour)) that escape only because of a
//     minus sign;
//   - AddDate(0, 0, 90) — literal days, not routed through TrialDays;
//   - Add(time.Duration(TrialDays)*24*time.Hour) or Add(trialLen) — a
//     duration bound to a variable before being passed to Add.
//
// A PASS does not prove trial end is defined in only one place; it only means
// this one pattern set hasn't found a second definition.
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

		// Not a trial end. This is the white-label app SUNSET schedule —
		// sunset_scheduled → downloads_blocked → pulled → firebase_archived —
		// which happens to use a 90-day step. Allowlisted with a reason rather
		// than renamed to dodge the regex: this guard exists to stop a second
		// definition of TRIAL end, not to ban 90-day durations.
		filepath.Join("internal", "whitelabel", "lifecycle", "advancer.go"): true,
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
			"that is how #353 happened. Call trial.EndsAt, or change EndsAt if this is genuinely a new definition, "+
			"or add the file to `allowed` with a comment if the duration is not a trial end at all.")
}
