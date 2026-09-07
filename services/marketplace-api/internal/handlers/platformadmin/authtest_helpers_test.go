package platformadmin_test

// Shared signed-request test fixtures. These used to live in
// middleware_test.go; that file (and its own tests) moved to
// github.com/mark8ly/platformauth (#720), but several handler/route tests
// in this package still build signed requests through RequirePlatformAuth,
// so the fixtures themselves stay here.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

const testSecret = "test-platform-secret"

var fixedNow = time.Unix(1755859200, 0).UTC()

// memNonces is an in-memory NonceStore, fine for tests that only exercise
// the enforcement matrix rather than the real cross-replica store.
type memNonces struct {
	seen          map[string]bool
	lastExpiresAt time.Time
}

func newMemNonces() *memNonces { return &memNonces{seen: map[string]bool{}} }

func (m *memNonces) Claim(_ context.Context, nonce string, expiresAt time.Time) (bool, error) {
	m.lastExpiresAt = expiresAt
	if m.seen[nonce] {
		return false, nil
	}
	m.seen[nonce] = true
	return true, nil
}

type reqOpt func(*platformadmin.SignatureInput)

func withoutOperator(in *platformadmin.SignatureInput)   { in.Operator = "" }
func withoutCapability(in *platformadmin.SignatureInput) { in.Capability = "" }

// withCapability overrides the signed capability value.
func withCapability(v string) reqOpt {
	return func(in *platformadmin.SignatureInput) { in.Capability = v }
}

func signedRequest(t *testing.T, method, target string, body []byte, opts ...reqOpt) *http.Request {
	t.Helper()

	// Split target into Path and RawQuery separately rather than assigning
	// the whole target to Path — the canonicaliser signs them as distinct
	// fields.
	parsed, err := url.Parse(target)
	require.NoError(t, err)

	in := platformadmin.SignatureInput{
		Method:     method,
		Path:       parsed.Path,
		RawQuery:   parsed.RawQuery,
		Body:       body,
		Timestamp:  "1755859200",
		Nonce:      uuid.NewString(),
		Operator:   "op_7f3a",
		Capability: "audit.read",
	}
	for _, o := range opts {
		o(&in)
	}

	sig, err := platformadmin.Sign(testSecret, in)
	require.NoError(t, err)

	var rdr *bytes.Reader
	if body == nil {
		rdr = bytes.NewReader(nil)
	} else {
		rdr = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, target, rdr)
	req.Header.Set(platformadmin.HeaderTimestamp, in.Timestamp)
	req.Header.Set(platformadmin.HeaderNonce, in.Nonce)
	req.Header.Set(platformadmin.HeaderSignature, sig)
	if in.Operator != "" {
		req.Header.Set(platformadmin.HeaderOperator, in.Operator)
	}
	if in.Capability != "" {
		req.Header.Set(platformadmin.HeaderCapability, in.Capability)
	}
	return req
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body.Error
}
