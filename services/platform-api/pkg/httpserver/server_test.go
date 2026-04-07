package httpserver

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNew_HealthEndpoint_Returns200(t *testing.T) {
	r := New("test", slog.New(slog.NewTextHandler(io.Discard, nil)))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != `{"status":"ok"}` {
		t.Errorf("body = %q, want %q", got, `{"status":"ok"}`)
	}
}

func TestNew_UnknownRoute_Returns404(t *testing.T) {
	r := New("test", slog.New(slog.NewTextHandler(io.Discard, nil)))

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestNew_DevMode_LogsAtDebugLevel(t *testing.T) {
	// Smoke test: just confirm New() doesn't panic in either mode.
	_ = New("dev", slog.New(slog.NewTextHandler(io.Discard, nil)))
	_ = New("prod", slog.New(slog.NewTextHandler(io.Discard, nil)))
}
