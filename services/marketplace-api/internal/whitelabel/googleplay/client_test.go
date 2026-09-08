package googleplay_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/whitelabel/googleplay"
)

// ─── test fixtures ───────────────────────────────────────────────────

// serviceAccountJSON builds a syntactically real service-account key with
// a freshly generated RSA private key, so golang.org/x/oauth2/google
// parses it and actually RS256-signs an assertion. A hand-written stub
// with a fake key would fail at signing time and every test below would
// pass for the wrong reason.
func serviceAccountJSON(t *testing.T, tokenURL string) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	body, err := json.Marshal(map[string]string{
		"type":         "service_account",
		"project_id":   "merchant-proj",
		"private_key":  string(pemBytes),
		"client_email": "teardown@merchant-proj.iam.gserviceaccount.com",
		"token_uri":    tokenURL,
	})
	if err != nil {
		t.Fatalf("marshal service account: %v", err)
	}
	return body
}

// playServer is a stand-in for Android Publisher plus Google's OAuth2
// token endpoint. Every status field defaults to 0, meaning "succeed".
type playServer struct {
	mu    sync.Mutex
	calls []string
	// Bodies received by PUT .../tracks/{track}, in order.
	trackUpdates []map[string]any

	// releases is what GET .../tracks/production reports.
	releases []map[string]any

	// Per-step failure injection. 0 = succeed.
	tokenStatus    int
	insertStatus   int
	trackGetStatus int
	updateStatus   int
	commitStatus   int
	deleteStatus   int
}

func (p *playServer) record(format string, args ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, fmt.Sprintf(format, args...))
}

func (p *playServer) recorded() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.calls...)
}

func (p *playServer) start(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("POST /token", func(w http.ResponseWriter, _ *http.Request) {
		p.record("token")
		if p.tokenStatus != 0 {
			// Shape of a real rejection for a revoked or deleted key.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(p.tokenStatus)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Invalid JWT Signature."}`))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"access_token": "test-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	})

	const appPath = "/androidpublisher/v3/applications/{pkg}/edits"

	mux.HandleFunc("POST "+appPath, func(w http.ResponseWriter, r *http.Request) {
		p.record("edits.insert %s", r.PathValue("pkg"))
		if p.insertStatus != 0 {
			writeAPIError(w, p.insertStatus)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": "edit-1"})
	})

	// The commit URL's last segment is "{editId}:commit", so a wildcard
	// captures the whole thing; the DELETE below shares the pattern and is
	// separated by method.
	mux.HandleFunc("POST "+appPath+"/{seg}", func(w http.ResponseWriter, r *http.Request) {
		p.record("edits.commit %s", r.PathValue("seg"))
		if p.commitStatus != 0 {
			writeAPIError(w, p.commitStatus)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": "edit-1"})
	})

	mux.HandleFunc("DELETE "+appPath+"/{editId}", func(w http.ResponseWriter, r *http.Request) {
		p.record("edits.delete %s", r.PathValue("editId"))
		if p.deleteStatus != 0 {
			writeAPIError(w, p.deleteStatus)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET "+appPath+"/{editId}/tracks/{track}", func(w http.ResponseWriter, r *http.Request) {
		p.record("edits.tracks.get %s", r.PathValue("track"))
		if p.trackGetStatus != 0 {
			writeAPIError(w, p.trackGetStatus)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"track":    r.PathValue("track"),
			"releases": p.releases,
		})
	})

	mux.HandleFunc("PUT "+appPath+"/{editId}/tracks/{track}", func(w http.ResponseWriter, r *http.Request) {
		p.record("edits.tracks.update %s", r.PathValue("track"))
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		p.mu.Lock()
		p.trackUpdates = append(p.trackUpdates, body)
		p.mu.Unlock()
		if p.updateStatus != 0 {
			writeAPIError(w, p.updateStatus)
			return
		}
		writeJSON(w, http.StatusOK, body)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeAPIError mimics a Google API error envelope so the generated
// client parses it into *googleapi.Error and the status code survives.
func writeAPIError(w http.ResponseWriter, status int) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    status,
			"message": http.StatusText(status),
			"status":  "ERROR",
		},
	})
}

// newClient wires a Client at srv for both the API and the token
// exchange, so no test reaches googleapis.com.
func newClient(t *testing.T, srv *httptest.Server) *googleplay.Client {
	t.Helper()
	tokenURL := srv.URL + "/token"
	cli, err := googleplay.New(googleplay.Config{
		Endpoint: srv.URL,
		TokenURL: tokenURL,
		HTTP:     srv.Client(),
		CredsFetcher: func(context.Context) (googleplay.Credentials, error) {
			return googleplay.Credentials{ServiceAccountJSON: serviceAccountJSON(t, tokenURL)}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return cli
}

func release(status string, extra map[string]any) map[string]any {
	r := map[string]any{"status": status, "versionCodes": []string{"41"}}
	for k, v := range extra {
		r[k] = v
	}
	return r
}

// ─── construction ────────────────────────────────────────────────────

func TestNew_RequiresCredsFetcher(t *testing.T) {
	if _, err := googleplay.New(googleplay.Config{}); err == nil {
		t.Error("New(empty) = nil, want error")
	}
}

func TestBlockDownloads_RejectsEmptyPackageName(t *testing.T) {
	ps := &playServer{}
	cli := newClient(t, ps.start(t))

	if err := cli.BlockDownloads(context.Background(), ""); err == nil {
		t.Fatal("BlockDownloads(\"\") = nil, want error")
	}
	// Rejected before anything is opened at Google — an edit inserted for
	// an empty package name would be a leak with no way to name it.
	if got := ps.recorded(); len(got) != 0 {
		t.Errorf("no request should be issued for an empty package; got %v", got)
	}
}

// ─── the edit lifecycle ──────────────────────────────────────────────

func TestBlockDownloads_HaltsProductionAndCommits(t *testing.T) {
	ps := &playServer{releases: []map[string]any{
		release("completed", nil),
	}}
	srv := ps.start(t)

	if err := newClient(t, srv).BlockDownloads(context.Background(), "com.example.store"); err != nil {
		t.Fatalf("BlockDownloads: %v", err)
	}

	want := []string{
		"token",
		"edits.insert com.example.store",
		"edits.tracks.get production",
		"edits.tracks.update production",
		"edits.commit edit-1:commit",
	}
	if got := ps.recorded(); !equalStrings(got, want) {
		t.Fatalf("call sequence =\n  %v\nwant\n  %v", got, want)
	}

	// The committed edit must actually carry status: halted — the whole
	// point of the step. Asserting only "update was called" would pass on
	// a body that changed nothing.
	if len(ps.trackUpdates) != 1 {
		t.Fatalf("track updates = %d, want 1", len(ps.trackUpdates))
	}
	if got := statusesIn(t, ps.trackUpdates[0]); !equalStrings(got, []string{"halted"}) {
		t.Errorf("committed release statuses = %v, want [halted]", got)
	}
}

func TestBlockDownloads_HaltsInProgressRolloutToo(t *testing.T) {
	ps := &playServer{releases: []map[string]any{
		release("inProgress", map[string]any{"userFraction": 0.1}),
	}}
	srv := ps.start(t)

	if err := newClient(t, srv).BlockDownloads(context.Background(), "com.example.store"); err != nil {
		t.Fatalf("BlockDownloads: %v", err)
	}
	if len(ps.trackUpdates) != 1 {
		t.Fatalf("track updates = %d, want 1", len(ps.trackUpdates))
	}
	if got := statusesIn(t, ps.trackUpdates[0]); !equalStrings(got, []string{"halted"}) {
		t.Errorf("statuses = %v, want [halted]", got)
	}
}

func TestBlockDownloads_LeavesDraftReleasesAlone(t *testing.T) {
	ps := &playServer{releases: []map[string]any{
		release("draft", nil),
		release("completed", nil),
	}}
	srv := ps.start(t)

	if err := newClient(t, srv).BlockDownloads(context.Background(), "com.example.store"); err != nil {
		t.Fatalf("BlockDownloads: %v", err)
	}
	if len(ps.trackUpdates) != 1 {
		t.Fatalf("track updates = %d, want 1", len(ps.trackUpdates))
	}
	// Draft is not served, so day 30 has no business changing it; halting
	// it would also discard the merchant's staged-but-unshipped work.
	if got := statusesIn(t, ps.trackUpdates[0]); !equalStrings(got, []string{"draft", "halted"}) {
		t.Errorf("statuses = %v, want [draft halted]", got)
	}
}

// ─── idempotency ─────────────────────────────────────────────────────

func TestBlockDownloads_AlreadyHalted_SucceedsWithoutCommitting(t *testing.T) {
	ps := &playServer{releases: []map[string]any{
		release("halted", nil),
	}}
	srv := ps.start(t)

	if err := newClient(t, srv).BlockDownloads(context.Background(), "com.example.store"); err != nil {
		t.Fatalf("BlockDownloads on an already-halted package = %v, want nil", err)
	}

	want := []string{
		"token",
		"edits.insert com.example.store",
		"edits.tracks.get production",
		"edits.delete edit-1",
	}
	if got := ps.recorded(); !equalStrings(got, want) {
		t.Fatalf("call sequence =\n  %v\nwant\n  %v", got, want)
	}
}

func TestBlockDownloads_NoReleasesAtAll_SucceedsWithoutCommitting(t *testing.T) {
	ps := &playServer{releases: nil}
	srv := ps.start(t)

	if err := newClient(t, srv).BlockDownloads(context.Background(), "com.example.store"); err != nil {
		t.Fatalf("BlockDownloads = %v, want nil", err)
	}
	if got := ps.recorded(); containsCall(got, "edits.commit") {
		t.Errorf("an empty track must not be committed; calls = %v", got)
	}
	if got := ps.recorded(); !containsCall(got, "edits.delete") {
		t.Errorf("the unused edit must be abandoned; calls = %v", got)
	}
}

// ─── the abandoned edit ──────────────────────────────────────────────

// A leaked edit is a real side effect on the merchant's Play account:
// Play permits one open edit at a time, so an abandoned one blocks the
// merchant's own releases until it expires. Every failure path after
// edits.insert must delete it — this table is the guard.
func TestBlockDownloads_DeletesTheEditOnEveryFailurePathAfterInsert(t *testing.T) {
	tests := []struct {
		name         string
		configure    func(*playServer)
		failedStep   string
		wantNoDelete bool
	}{
		{
			name:       "tracks.get fails",
			configure:  func(p *playServer) { p.trackGetStatus = http.StatusInternalServerError },
			failedStep: "edits.tracks.get",
		},
		{
			name:       "tracks.update fails",
			configure:  func(p *playServer) { p.updateStatus = http.StatusInternalServerError },
			failedStep: "edits.tracks.update",
		},
		{
			name:       "commit fails",
			configure:  func(p *playServer) { p.commitStatus = http.StatusInternalServerError },
			failedStep: "edits.commit",
		},
		{
			name:       "credentials rejected mid-lifecycle",
			configure:  func(p *playServer) { p.trackGetStatus = http.StatusForbidden },
			failedStep: "edits.tracks.get",
		},
		{
			// The insert itself failing is the one case with nothing to
			// clean up: no edit was created, so a DELETE would be a
			// request against an id the client never received.
			name:         "insert fails — nothing to abandon",
			configure:    func(p *playServer) { p.insertStatus = http.StatusInternalServerError },
			failedStep:   "edits.insert",
			wantNoDelete: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := &playServer{releases: []map[string]any{release("completed", nil)}}
			tc.configure(ps)
			srv := ps.start(t)

			err := newClient(t, srv).BlockDownloads(context.Background(), "com.example.store")
			if err == nil {
				t.Fatal("BlockDownloads = nil, want error")
			}
			if !strings.Contains(err.Error(), tc.failedStep) {
				t.Errorf("err = %v, want it to name %s", err, tc.failedStep)
			}

			deleted := containsCall(ps.recorded(), "edits.delete")
			if tc.wantNoDelete && deleted {
				t.Errorf("edits.delete was called with no edit to delete; calls = %v", ps.recorded())
			}
			if !tc.wantNoDelete && !deleted {
				t.Errorf("edit leaked — no edits.delete; calls = %v", ps.recorded())
			}
		})
	}
}

// A cleanup that fails must not be silent: the leak needs an operator, and
// this package has no logger, so the delete failure rides back on the
// returned error alongside the original cause.
func TestBlockDownloads_FailedCleanupIsReportedAlongsideTheCause(t *testing.T) {
	ps := &playServer{
		releases:       []map[string]any{release("completed", nil)},
		trackGetStatus: http.StatusInternalServerError,
		deleteStatus:   http.StatusInternalServerError,
	}
	srv := ps.start(t)

	err := newClient(t, srv).BlockDownloads(context.Background(), "com.example.store")
	if err == nil {
		t.Fatal("BlockDownloads = nil, want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "edits.tracks.get") {
		t.Errorf("err = %v, want the original cause", err)
	}
	if !strings.Contains(msg, "edits.delete") || !strings.Contains(msg, "edit-1") {
		t.Errorf("err = %v, want the leaked edit id reported too", err)
	}
}

// A successful commit consumes the edit, so deleting it afterwards would
// be a spurious request against an id that no longer exists.
func TestBlockDownloads_DoesNotDeleteACommittedEdit(t *testing.T) {
	ps := &playServer{releases: []map[string]any{release("completed", nil)}}
	srv := ps.start(t)

	if err := newClient(t, srv).BlockDownloads(context.Background(), "com.example.store"); err != nil {
		t.Fatalf("BlockDownloads: %v", err)
	}
	if containsCall(ps.recorded(), "edits.delete") {
		t.Errorf("a committed edit must not be deleted; calls = %v", ps.recorded())
	}
}

// ─── auth failures are distinguishable ───────────────────────────────

func TestBlockDownloads_RevokedKey_IsAnAuthFailureNotATransportFailure(t *testing.T) {
	// A deleted or disabled service-account key fails at the token
	// exchange, before any API request is sent.
	ps := &playServer{tokenStatus: http.StatusBadRequest}
	srv := ps.start(t)

	err := newClient(t, srv).BlockDownloads(context.Background(), "com.example.store")
	if err == nil {
		t.Fatal("BlockDownloads = nil, want error")
	}
	if !isUnauthorized(err) {
		t.Errorf("err = %v, want wraps ErrUnauthorized", err)
	}
	if containsCall(ps.recorded(), "edits.insert") {
		t.Errorf("no API request should be sent once the token exchange fails; calls = %v", ps.recorded())
	}
}

func TestBlockDownloads_ForbiddenFromTheAPI_IsAnAuthFailure(t *testing.T) {
	// The key signs fine but the service account is not linked to this
	// developer account, or lacks the release-manager role.
	ps := &playServer{insertStatus: http.StatusForbidden}
	srv := ps.start(t)

	err := newClient(t, srv).BlockDownloads(context.Background(), "com.example.store")
	if !isUnauthorized(err) {
		t.Errorf("err = %v, want wraps ErrUnauthorized", err)
	}
}

func TestBlockDownloads_ServerErrorIsNotAnAuthFailure(t *testing.T) {
	ps := &playServer{insertStatus: http.StatusServiceUnavailable}
	srv := ps.start(t)

	err := newClient(t, srv).BlockDownloads(context.Background(), "com.example.store")
	if err == nil {
		t.Fatal("BlockDownloads = nil, want error")
	}
	// A 503 resolves itself on the next tick; classifying it as a
	// credential problem would send an operator to re-onboard a key that
	// is perfectly good.
	if isUnauthorized(err) {
		t.Errorf("err = %v, must NOT wrap ErrUnauthorized", err)
	}
}

func TestBlockDownloads_UnreachableHostIsNotAnAuthFailure(t *testing.T) {
	ps := &playServer{}
	srv := ps.start(t)
	url := srv.URL
	srv.Close() // nothing is listening now

	cli, err := googleplay.New(googleplay.Config{
		Endpoint: url,
		TokenURL: url + "/token",
		CredsFetcher: func(context.Context) (googleplay.Credentials, error) {
			return googleplay.Credentials{ServiceAccountJSON: serviceAccountJSON(t, url+"/token")}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	blockErr := cli.BlockDownloads(context.Background(), "com.example.store")
	if blockErr == nil {
		t.Fatal("BlockDownloads = nil, want error")
	}
	if isUnauthorized(blockErr) {
		t.Errorf("err = %v, must NOT wrap ErrUnauthorized", blockErr)
	}
}

func TestBlockDownloads_MalformedServiceAccountJSONIsAnAuthFailure(t *testing.T) {
	cli, err := googleplay.New(googleplay.Config{
		CredsFetcher: func(context.Context) (googleplay.Credentials, error) {
			return googleplay.Credentials{ServiceAccountJSON: []byte(`{"type":"authorized_user"}`)}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := cli.BlockDownloads(context.Background(), "com.example.store"); !isUnauthorized(got) {
		t.Errorf("err = %v, want wraps ErrUnauthorized", got)
	}
}

func TestBlockDownloads_EmptyServiceAccountJSONIsAnAuthFailure(t *testing.T) {
	cli, err := googleplay.New(googleplay.Config{
		CredsFetcher: func(context.Context) (googleplay.Credentials, error) {
			return googleplay.Credentials{}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := cli.BlockDownloads(context.Background(), "com.example.store"); !isUnauthorized(got) {
		t.Errorf("err = %v, want wraps ErrUnauthorized", got)
	}
}

func TestBlockDownloads_CredsFetcherErrorSurfaces(t *testing.T) {
	sentinel := errors.New("creds read failed")
	cli, err := googleplay.New(googleplay.Config{
		CredsFetcher: func(context.Context) (googleplay.Credentials, error) {
			return googleplay.Credentials{}, sentinel
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := cli.BlockDownloads(context.Background(), "com.example"); !errors.Is(got, sentinel) {
		t.Errorf("err = %v, want wraps the appcreds failure", got)
	}
}

// ─── PullApp is honest ───────────────────────────────────────────────

// Unpublishing a Play listing has no Android Publisher API. PullApp must
// say so — not succeed, and not quietly do the day-30 halt instead.
func TestPullApp_ReportsThatUnpublishingHasNoAPI(t *testing.T) {
	ps := &playServer{releases: []map[string]any{release("completed", nil)}}
	srv := ps.start(t)

	credsRead := 0
	cli, err := googleplay.New(googleplay.Config{
		Endpoint: srv.URL,
		TokenURL: srv.URL + "/token",
		HTTP:     srv.Client(),
		CredsFetcher: func(context.Context) (googleplay.Credentials, error) {
			credsRead++
			return googleplay.Credentials{ServiceAccountJSON: serviceAccountJSON(t, srv.URL+"/token")}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	pullErr := cli.PullApp(context.Background(), "com.example.store")
	if pullErr == nil {
		t.Fatal("PullApp = nil; a listing that is still up must not report success")
	}
	if !errors.Is(pullErr, googleplay.ErrUnpublishNotSupported) {
		t.Errorf("err = %v, want wraps ErrUnpublishNotSupported", pullErr)
	}
	if !strings.Contains(pullErr.Error(), "com.example.store") {
		t.Errorf("err = %v, want it to name the package so the record is actionable", pullErr)
	}

	// No request, and no credential read: an appcreds access would put an
	// audit entry on the merchant's key implying an attempt was made.
	if got := ps.recorded(); len(got) != 0 {
		t.Errorf("PullApp issued requests: %v", got)
	}
	if credsRead != 0 {
		t.Errorf("credentials read %d times; want 0", credsRead)
	}
}

// The constraint is PERMANENT, and it must not read as pending work.
// firebase.ErrNotWired one package over means the opposite — "an
// implementation is coming" — and a reader who takes this for that waits
// for a follow-up PR that can never be written. So the error is
// ErrUnpublishNotSupported and its text says what stands in the way, with
// no "not yet"/"not wired" wording anywhere in it.
func TestPullApp_ReportsAPermanentConstraintNotPendingWork(t *testing.T) {
	cli, err := googleplay.New(googleplay.Config{
		CredsFetcher: func(context.Context) (googleplay.Credentials, error) {
			return googleplay.Credentials{}, nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got := cli.PullApp(context.Background(), "com.example.store")
	if !errors.Is(got, googleplay.ErrUnpublishNotSupported) {
		t.Fatalf("err = %v, want wraps ErrUnpublishNotSupported", got)
	}
	msg := strings.ToLower(got.Error())
	for _, pending := range []string{"not yet", "not wired", "yet to be", "follow-up", "todo"} {
		if strings.Contains(msg, pending) {
			t.Errorf("err = %q contains %q, which reads as pending work", got, pending)
		}
	}
}

// ─── an unrecognised status must not pass for a halt ─────────────────

// Copying an unknown status through would leave nothing changed, which
// makes BlockDownloads abandon the edit and return nil — and the advancer
// then writes downloads_blocked for an app that may still be serving.
// That is a step reporting success it never performed.
func TestBlockDownloads_UnrecognisedReleaseStatus_FailsInsteadOfReportingSuccess(t *testing.T) {
	tests := []struct {
		name   string
		status string
	}{
		{"a status this code has never seen", "someFutureStatus"},
		// statusUnspecified is the enum's fifth value. It says nothing
		// about whether APKs are served, so it cannot be assumed harmless.
		{"statusUnspecified", "statusUnspecified"},
		{"empty status", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ps := &playServer{releases: []map[string]any{
				release(tc.status, map[string]any{"name": "41 (1.4.1)"}),
			}}
			srv := ps.start(t)

			err := newClient(t, srv).BlockDownloads(context.Background(), "com.example.store")
			if err == nil {
				t.Fatal("BlockDownloads = nil; an unhaltable release must not report success")
			}
			if !strings.Contains(err.Error(), tc.status) {
				t.Errorf("err = %v, want it to name the offending status %q", err, tc.status)
			}
			if len(ps.trackUpdates) != 0 {
				t.Errorf("nothing should be written to the track; got %v", ps.trackUpdates)
			}
			if containsCall(ps.recorded(), "edits.commit") {
				t.Errorf("the edit must not be committed; calls = %v", ps.recorded())
			}
			if !containsCall(ps.recorded(), "edits.delete") {
				t.Errorf("the edit leaked; calls = %v", ps.recorded())
			}
		})
	}
}

// A geo-staged rollout is a case the inProgress branch claims to handle.
// countryTargeting is documented as settable only for inProgress
// production releases, so round-tripping it onto a now-halted release can
// fail the update on that field alone. Clearing it widens nothing — a
// halted release serves nowhere.
func TestBlockDownloads_ClearsCountryTargetingWhenHaltingAGeoStagedRollout(t *testing.T) {
	ps := &playServer{releases: []map[string]any{
		release("inProgress", map[string]any{
			"userFraction":     0.25,
			"countryTargeting": map[string]any{"countries": []string{"US", "CA"}},
		}),
	}}
	srv := ps.start(t)

	if err := newClient(t, srv).BlockDownloads(context.Background(), "com.example.store"); err != nil {
		t.Fatalf("BlockDownloads: %v", err)
	}
	if len(ps.trackUpdates) != 1 {
		t.Fatalf("track updates = %d, want 1", len(ps.trackUpdates))
	}
	rel := firstRelease(t, ps.trackUpdates[0])
	if rel["status"] != "halted" {
		t.Errorf("status = %v, want halted", rel["status"])
	}
	if _, present := rel["countryTargeting"]; present {
		t.Errorf("countryTargeting survived onto a halted release: %#v", rel)
	}
}

// ─── fake ────────────────────────────────────────────────────────────

func TestFakeClient_Counts(t *testing.T) {
	f := googleplay.NewFakeClient()
	_ = f.BlockDownloads(context.Background(), "com.a")
	_ = f.PullApp(context.Background(), "com.a")
	_ = f.PullApp(context.Background(), "com.b")
	if f.BlockDownloadsCallCount != 1 {
		t.Errorf("Block count = %d, want 1", f.BlockDownloadsCallCount)
	}
	if f.PullAppCallCount != 2 {
		t.Errorf("Pull count = %d, want 2", f.PullAppCallCount)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────

func statusesIn(t *testing.T, body map[string]any) []string {
	t.Helper()
	raw, ok := body["releases"].([]any)
	if !ok {
		t.Fatalf("update body has no releases array: %#v", body)
	}
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		rel, ok := r.(map[string]any)
		if !ok {
			t.Fatalf("release is not an object: %#v", r)
		}
		status, _ := rel["status"].(string)
		out = append(out, status)
	}
	return out
}

// isUnauthorized is the check a caller makes to decide "re-onboard this
// merchant's key" versus "retry next tick".
func isUnauthorized(err error) bool {
	return errors.Is(err, googleplay.ErrUnauthorized)
}

func firstRelease(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	raw, ok := body["releases"].([]any)
	if !ok || len(raw) == 0 {
		t.Fatalf("update body has no releases: %#v", body)
	}
	rel, ok := raw[0].(map[string]any)
	if !ok {
		t.Fatalf("release is not an object: %#v", raw[0])
	}
	return rel
}

func containsCall(calls []string, prefix string) bool {
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
