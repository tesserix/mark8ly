package platformadmin_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// authoritativeCodes is the SAME data the handler builds from, restated here
// as the four exported vars rather than as a literal copy.
//
// Restating the literals would make this file the fifth place the vocabulary
// lives, which is the failure the endpoint exists to end. Reading the exported
// vars means these tests assert the RELATIONSHIP — every code labelled, every
// label used, every code published — and stay silent about membership, which
// is the writers' business.
func authoritativeCodes() map[string][]string {
	return map[string][]string{
		"suspend":      platformadmin.SuspendReasonCodes,
		"unsuspend":    platformadmin.UnsuspendReasonCodes,
		"trial_extend": platformadmin.ExtendReasonCodes,
		"purge":        platformadmin.PurgeReasonCodes,
	}
}

// TestReasonCodeLabelsCoverEveryCode is the drift guard the label map's
// comment promises.
//
// The map and the `[]string` lists are two structures describing one
// vocabulary, which is a drift waiting to happen. It cannot happen silently:
// adding a code without a label fails here, and the fallback that would
// otherwise ship the bare `tos_violation` to an operator never gets to fire in
// production.
func TestReasonCodeLabelsCoverEveryCode(t *testing.T) {
	published := platformadmin.LifecycleReasonCodes()

	for verb, codes := range authoritativeCodes() {
		entries, ok := published[verb]
		require.Truef(t, ok, "verb %q has codes but is not published", verb)
		require.Lenf(t, entries, len(codes), "verb %q publishes a different number of codes than it validates", verb)

		for i, code := range codes {
			require.Equalf(t, code, entries[i].Code,
				"verb %q entry %d: published order must follow the authoritative list", verb, i)
			require.NotEmptyf(t, entries[i].Label, "verb %q code %q has no label", verb, code)
			require.NotEqualf(t, code, entries[i].Label,
				"verb %q code %q fell back to its own code as a label — add it to reasonCodeLabels", verb, code)
		}
	}
}

// The mirror of the test above: a label for a code that no longer exists is
// dead weight that reads as though the code is still accepted.
func TestNoOrphanedReasonCodeLabels(t *testing.T) {
	published := platformadmin.LifecycleReasonCodes()
	for verb := range published {
		_, ok := authoritativeCodes()[verb]
		require.Truef(t, ok, "verb %q is published but has no authoritative code list", verb)
	}
	require.Len(t, published, len(authoritativeCodes()),
		"every verb with codes must be published, and nothing else")
}

// `operator_error` is the reason the label map is nested per verb rather than
// flat. It means three different things, and a flat map would have shown an
// operator the wrong sentence beside the right code.
func TestSharedCodesCarryVerbSpecificLabels(t *testing.T) {
	published := platformadmin.LifecycleReasonCodes()

	labelOf := func(verb, code string) string {
		for _, entry := range published[verb] {
			if entry.Code == code {
				return entry.Label
			}
		}
		t.Fatalf("verb %q does not publish %q", verb, code)
		return ""
	}

	unsuspend := labelOf("unsuspend", "operator_error")
	extend := labelOf("trial_extend", "operator_error")
	purge := labelOf("purge", "operator_error")

	require.NotEqual(t, unsuspend, extend, "operator_error must not read the same on unsuspend and trial_extend")
	require.NotEqual(t, extend, purge, "operator_error must not read the same on trial_extend and purge")
	require.NotEqual(t, unsuspend, purge, "operator_error must not read the same on unsuspend and purge")
}

// §8.8 pins snake_case because the code lands verbatim in an audit row and is
// matched exactly by isKnownReasonCode. A product serving `Non-Payment` would
// pass its own validator and break the moment two products' rows were read
// together.
func TestPublishedCodesAreSnakeCase(t *testing.T) {
	pattern := regexp.MustCompile(`^[a-z0-9]+(?:_[a-z0-9]+)*$`)
	for verb, entries := range platformadmin.LifecycleReasonCodes() {
		for _, entry := range entries {
			require.Truef(t, pattern.MatchString(entry.Code),
				"verb %q code %q is not snake_case (contract §8.8)", verb, entry.Code)
		}
	}
}

func TestReasonCodesEndpointServesTheContractShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	platformadmin.NewLifecycleReasonCodesHandler().Register(router.Group(""))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/lifecycle/reason-codes", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	// Decoded into the wire shape rather than the handler's own struct: the
	// contract is about what a console reading raw JSON sees, and unmarshalling
	// into the producer's type would hide a wrong or missing json tag.
	var body struct {
		Data map[string][]struct {
			Code  string `json:"code"`
			Label string `json:"label"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

	// §8.8 requires both verbs of every implementer and permits more.
	require.Contains(t, body.Data, "suspend")
	require.Contains(t, body.Data, "unsuspend")
	require.NotEmpty(t, body.Data["suspend"])
	require.NotEmpty(t, body.Data["unsuspend"])

	for verb, entries := range body.Data {
		for i, entry := range entries {
			require.NotEmptyf(t, entry.Code, "%s[%d] has no code", verb, i)
			require.NotEmptyf(t, entry.Label, "%s[%d] has no label", verb, i)
		}
	}
}

// The suspend and unsuspend sets are deliberately different — the reason a
// suspension ends is not the reason it began — and a refactor that collapsed
// them into one list would be invisible to every other test here.
func TestSuspendAndUnsuspendVocabulariesStayDistinct(t *testing.T) {
	published := platformadmin.LifecycleReasonCodes()

	suspend := map[string]bool{}
	for _, entry := range published["suspend"] {
		suspend[entry.Code] = true
	}
	for _, entry := range published["unsuspend"] {
		require.Falsef(t, suspend[entry.Code],
			"unsuspend code %q also appears in suspend; the two sets are deliberately disjoint", entry.Code)
	}
}
