package onboardingfunnel_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/onboardingfunnel"
)

func TestGetFunnelSendsAuthAndParsesEnvelope(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("X-Internal-Auth")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{
			"started":10,
			"email_verified":8,
			"completed":5,
			"in_flight":3,
			"abandoned":2,
			"median_completion_seconds":123.5,
			"last_24h":{"started":4,"email_verified":3,"completed":2,"in_flight":1,"abandoned":1},
			"window":{"from":"2026-08-01T00:00:00Z","to":"2026-08-23T00:00:00Z"}
		}}`))
	}))
	defer srv.Close()

	c := onboardingfunnel.NewClient(srv.URL, "s3cret", srv.Client())
	got, err := c.GetFunnel(context.Background(), onboardingfunnel.FunnelParams{})
	require.NoError(t, err)

	require.Equal(t, "s3cret", gotAuth)
	require.Equal(t, "/internal/onboarding/funnel", gotPath)
	require.Equal(t, int64(10), got.Started)
	require.Equal(t, int64(8), got.EmailVerified)
	require.Equal(t, int64(5), got.Completed)
	require.Equal(t, int64(3), got.InFlight)
	require.Equal(t, int64(2), got.Abandoned)
	require.NotNil(t, got.MedianCompletionSeconds)
	require.Equal(t, 123.5, *got.MedianCompletionSeconds)
	require.Equal(t, int64(4), got.Last24h.Started)
	require.Equal(t, int64(2), got.Last24h.Completed)
	require.Equal(t, "2026-08-01T00:00:00Z", got.Window.From)
	require.Equal(t, "2026-08-23T00:00:00Z", got.Window.To)
}

// null must survive as Go nil, not become 0 ("instant completion").
func TestGetFunnelMedianCompletionSecondsNull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{
			"started":0,
			"email_verified":0,
			"completed":0,
			"in_flight":0,
			"abandoned":0,
			"median_completion_seconds":null,
			"last_24h":{"started":0,"email_verified":0,"completed":0,"in_flight":0,"abandoned":0},
			"window":{"from":"2026-08-01T00:00:00Z","to":"2026-08-23T00:00:00Z"}
		}}`))
	}))
	defer srv.Close()

	c := onboardingfunnel.NewClient(srv.URL, "s", srv.Client())
	got, err := c.GetFunnel(context.Background(), onboardingfunnel.FunnelParams{})
	require.NoError(t, err)
	require.Nil(t, got.MedianCompletionSeconds)
}

func TestListSessionsParsesEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"s1","email":"a@example.com","status":"completed","last_activity_at":"2026-08-22T10:00:00Z","created_at":"2026-08-22T09:00:00Z","abandoned":false}],"pagination":{"page":1,"limit":50,"total":1}}`))
	}))
	defer srv.Close()

	c := onboardingfunnel.NewClient(srv.URL, "s", srv.Client())
	got, err := c.ListSessions(context.Background(), onboardingfunnel.SessionsParams{})
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Total)
	require.Len(t, got.Sessions, 1)
	require.Equal(t, "s1", got.Sessions[0].ID)
}

func TestListSessionsEmptyIsAllocated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"page":1,"limit":50,"total":0}}`))
	}))
	defer srv.Close()

	got, err := onboardingfunnel.NewClient(srv.URL, "s", srv.Client()).
		ListSessions(context.Background(), onboardingfunnel.SessionsParams{})
	require.NoError(t, err)
	require.NotNil(t, got.Sessions)
	require.Empty(t, got.Sessions)
}

func TestListSessionsNullDataIsAllocated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":null,"pagination":{"page":1,"limit":50,"total":0}}`))
	}))
	defer srv.Close()

	got, err := onboardingfunnel.NewClient(srv.URL, "s", srv.Client()).
		ListSessions(context.Background(), onboardingfunnel.SessionsParams{})
	require.NoError(t, err)
	require.NotNil(t, got.Sessions)
	require.Empty(t, got.Sessions)
}

func TestGetFunnelUpstream5xxIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := onboardingfunnel.NewClient(srv.URL, "s", srv.Client()).
		GetFunnel(context.Background(), onboardingfunnel.FunnelParams{})
	require.ErrorIs(t, err, onboardingfunnel.ErrUnavailable)
}

func TestGetFunnelTransportFailureIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close() // closed: connection refused

	_, err := onboardingfunnel.NewClient(srv.URL, "s", srv.Client()).
		GetFunnel(context.Background(), onboardingfunnel.FunnelParams{})
	require.ErrorIs(t, err, onboardingfunnel.ErrUnavailable)
}

func TestListSessionsUpstream5xxIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := onboardingfunnel.NewClient(srv.URL, "s", srv.Client()).
		ListSessions(context.Background(), onboardingfunnel.SessionsParams{})
	require.ErrorIs(t, err, onboardingfunnel.ErrUnavailable)
}

func TestListSessionsTransportFailureIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close() // closed: connection refused

	_, err := onboardingfunnel.NewClient(srv.URL, "s", srv.Client()).
		ListSessions(context.Background(), onboardingfunnel.SessionsParams{})
	require.ErrorIs(t, err, onboardingfunnel.ErrUnavailable)
}

// RFC3339 offsets contain "+" (e.g. +05:30). A naive string concatenation of
// the window params corrupts it into a space; only proper URL encoding via
// url.Values survives the round trip.
func TestWindowParamsAreURLEncoded(t *testing.T) {
	loc := time.FixedZone("IST", 5*60*60+30*60)
	from := time.Date(2026, 8, 1, 9, 30, 0, 0, loc)
	to := time.Date(2026, 8, 23, 18, 0, 0, 0, loc)

	var gotFrom, gotTo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFrom = r.URL.Query().Get("created_from")
		gotTo = r.URL.Query().Get("created_to")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{
			"started":0,"email_verified":0,"completed":0,"in_flight":0,"abandoned":0,
			"median_completion_seconds":null,
			"last_24h":{"started":0,"email_verified":0,"completed":0,"in_flight":0,"abandoned":0},
			"window":{"from":"2026-08-01T00:00:00Z","to":"2026-08-23T00:00:00Z"}
		}}`))
	}))
	defer srv.Close()

	c := onboardingfunnel.NewClient(srv.URL, "s", srv.Client())
	_, err := c.GetFunnel(context.Background(), onboardingfunnel.FunnelParams{
		CreatedFrom: from,
		CreatedTo:   to,
	})
	require.NoError(t, err)

	require.Equal(t, from.Format(time.RFC3339), gotFrom)
	require.Equal(t, to.Format(time.RFC3339), gotTo)
	require.Contains(t, gotFrom, "+05:30")
	require.Contains(t, gotTo, "+05:30")
}
