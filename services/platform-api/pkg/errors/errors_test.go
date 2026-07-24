package errors

import (
	"errors"
	"net/http"
	"testing"
)

func TestNew_SetsCodeMessageStatus(t *testing.T) {
	e := New(http.StatusBadRequest, "bad_input", "missing field")
	if e.Code != "bad_input" {
		t.Errorf("Code = %q, want %q", e.Code, "bad_input")
	}
	if e.Message != "missing field" {
		t.Errorf("Message = %q, want %q", e.Message, "missing field")
	}
	if e.Status != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", e.Status, http.StatusBadRequest)
	}
}

func TestError_FormatsCodeAndMessage(t *testing.T) {
	e := New(http.StatusNotFound, "tenant_not_found", "no such tenant")
	want := "tenant_not_found: no such tenant"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestError_IncludesWrappedError(t *testing.T) {
	cause := errors.New("connection refused")
	e := Wrap(cause, http.StatusInternalServerError, "db_error", "open db")
	want := "db_error: open db: connection refused"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestUnwrap_ReturnsWrappedError(t *testing.T) {
	cause := errors.New("inner")
	e := Wrap(cause, http.StatusInternalServerError, "x", "y")
	if !errors.Is(e, cause) {
		t.Errorf("errors.Is should return true for wrapped cause")
	}
}

func TestAs_ExtractsAppErrorFromWrappedChain(t *testing.T) {
	inner := New(http.StatusConflict, "duplicate", "already exists")
	outer := errors.Join(errors.New("context"), inner)

	got, ok := As(outer)
	if !ok {
		t.Fatalf("As() should return true when chain contains AppError")
	}
	if got.Code != "duplicate" {
		t.Errorf("extracted Code = %q, want %q", got.Code, "duplicate")
	}
}

func TestAs_ReturnsFalseForNonAppError(t *testing.T) {
	plain := errors.New("just an error")
	if _, ok := As(plain); ok {
		t.Error("As() should return false for non-AppError")
	}
}

func TestHelpers_ReturnExpectedHTTPStatuses(t *testing.T) {
	cases := []struct {
		name string
		fn   func(string, string) *AppError
		want int
	}{
		{"NotFound", NotFound, http.StatusNotFound},
		{"BadRequest", BadRequest, http.StatusBadRequest},
		{"Conflict", Conflict, http.StatusConflict},
		{"Internal", Internal, http.StatusInternalServerError},
		{"Unauthorized", Unauthorized, http.StatusUnauthorized},
		{"Forbidden", Forbidden, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := tc.fn("code", "msg")
			if e.Status != tc.want {
				t.Errorf("%s status = %d, want %d", tc.name, e.Status, tc.want)
			}
		})
	}
}
