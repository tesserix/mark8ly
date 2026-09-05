package platformadmin_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// The consumer is tesserix-home/platform-api. Three layers agree that
// `metadata` is a STRING carrying compact JSON, not an object:
//
//   - internal/modules/audit/internal/domain/entry.go — Metadata string, and
//     that struct IS the federation decode target
//   - apps/console/lib/audit.ts:196 — optionalStr(row.metadata, ...)
//   - the console's own writer renders compact JSON by hand into the column
//
// Getting this wrong is not cosmetic. The consumer decodes a whole page with
// one json.Unmarshal, so an object where a string is expected fails the
// decode, and mark8ly becomes a federation FAILURE — every mark8ly audit row
// vanishes from the console, not just this field. See #313.

func metadataRouter(t *testing.T, entries []audit.Entry) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := &stubRepo{result: audit.ListResult{Total: int64(len(entries)), Entries: entries}}
	r := gin.New()
	platformadmin.NewAuditLogsHandler(nil, repo, nil).Register(r.Group(""))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/audit-logs", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	return rec
}

// rawRows decodes data[] without imposing a type on metadata, so the test can
// assert what the field actually IS on the wire.
func rawRows(t *testing.T, rec *httptest.ResponseRecorder) []map[string]json.RawMessage {
	t.Helper()
	var body struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body.Data
}

func entryWithMetadata(m audit.Metadata) audit.Entry {
	return audit.Entry{
		ID:           uuid.MustParse("3f2504e0-4f89-11d3-9a0c-0305e82c3301"),
		TenantID:     uuid.New(),
		ActorType:    audit.ActorOperator,
		Action:       "subscription.plan_changed",
		ResourceType: "subscription",
		CreatedAt:    time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC),
		Metadata:     m,
	}
}

// The whole point of #313: metadata is a JSON string, never an object.
func TestMetadataIsAJSONStringNotAnObject(t *testing.T) {
	rec := metadataRouter(t, []audit.Entry{
		entryWithMetadata(audit.Metadata{"plan": "pro", "previous_plan": "starter"}),
	})

	rows := rawRows(t, rec)
	require.Len(t, rows, 1)
	raw, ok := rows[0]["metadata"]
	require.True(t, ok, "metadata must be present when the entry has some")

	// A string on the wire — it unmarshals into a string, and the raw bytes
	// are quoted rather than starting a JSON object.
	var s string
	require.NoError(t, json.Unmarshal(raw, &s),
		"metadata must decode as a string; an object here breaks the consumer's whole page")
	assert.Equal(t, byte('"'), raw[0])

	// And its CONTENT is the compact JSON of the stored map.
	var round map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &round))
	assert.Equal(t, map[string]any{"plan": "pro", "previous_plan": "starter"}, round)
	assert.NotContains(t, s, " ", "compact, not indented")
}

// Empty and absent metadata must be OMITTED, not sent as "{}". "No metadata"
// and "metadata containing nothing" are different facts, and the contract
// marks the field optional.
func TestEmptyMetadataIsOmittedNotEmptyObjectString(t *testing.T) {
	for name, m := range map[string]audit.Metadata{
		"nil":   nil,
		"empty": {},
	} {
		t.Run(name, func(t *testing.T) {
			rows := rawRows(t, metadataRouter(t, []audit.Entry{entryWithMetadata(m)}))
			require.Len(t, rows, 1)
			_, present := rows[0]["metadata"]
			assert.False(t, present, "metadata must be omitted, not sent as an empty string or {}")
		})
	}
}

// Key order must be stable, or every poll shows a diff for rows that did not
// change. encoding/json sorts map keys; this pins that we depend on it.
func TestMetadataKeyOrderIsDeterministic(t *testing.T) {
	m := audit.Metadata{"zebra": 1, "alpha": 2, "middle": 3}
	first := rawRows(t, metadataRouter(t, []audit.Entry{entryWithMetadata(m)}))[0]["metadata"]
	for range 5 {
		got := rawRows(t, metadataRouter(t, []audit.Entry{entryWithMetadata(m)}))[0]["metadata"]
		assert.JSONEq(t, string(first), string(got))
		assert.Equal(t, string(first), string(got), "byte-identical, so a poller sees no phantom change")
	}
	var s string
	require.NoError(t, json.Unmarshal(first, &s))
	assert.Equal(t, `{"alpha":2,"middle":3,"zebra":1}`, s)
}

// Nested values survive: the column is jsonb and some rows carry structure.
func TestMetadataPreservesNestedValues(t *testing.T) {
	rows := rawRows(t, metadataRouter(t, []audit.Entry{
		entryWithMetadata(audit.Metadata{
			"refund": map[string]any{"amount": 1250, "currency": "aud"},
			"items":  []any{"a", "b"},
		}),
	}))
	var s string
	require.NoError(t, json.Unmarshal(rows[0]["metadata"], &s))

	var round map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &round))
	assert.Equal(t, float64(1250), round["refund"].(map[string]any)["amount"])
	assert.Equal(t, []any{"a", "b"}, round["items"])
}

// One unserialisable row must not take the page down with it: the rest of the
// rows still render, and the offender simply has no metadata.
func TestUnserialisableMetadataOmitsTheFieldAndKeepsThePage(t *testing.T) {
	bad := entryWithMetadata(audit.Metadata{"ch": make(chan int)})
	good := entryWithMetadata(audit.Metadata{"plan": "pro"})
	good.ID = uuid.MustParse("3f2504e0-4f89-11d3-9a0c-0305e82c3302")

	rows := rawRows(t, metadataRouter(t, []audit.Entry{bad, good}))
	require.Len(t, rows, 2, "a bad row must not truncate the page")

	_, present := rows[0]["metadata"]
	assert.False(t, present, "unserialisable metadata is dropped, not partially emitted")

	var s string
	require.NoError(t, json.Unmarshal(rows[1]["metadata"], &s))
	assert.JSONEq(t, `{"plan":"pro"}`, s)
}
