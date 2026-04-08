package httpserver

import (
	"io"
	"log/slog"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/mode"
)

func TestNew_Admin_PopulatesOnlyAdminEngine(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	e := New("test", mode.Admin, log)
	if e.Admin == nil {
		t.Error("Admin engine should be non-nil for mode.Admin")
	}
	if e.Storefront != nil {
		t.Error("Storefront engine should be nil for mode.Admin")
	}
}

func TestNew_Storefront_PopulatesOnlyStorefrontEngine(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	e := New("test", mode.Storefront, log)
	if e.Admin != nil {
		t.Error("Admin engine should be nil for mode.Storefront")
	}
	if e.Storefront == nil {
		t.Error("Storefront engine should be non-nil for mode.Storefront")
	}
}

func TestMergedForBoth_ReturnsNonNil(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if MergedForBoth("test", log) == nil {
		t.Error("MergedForBoth should return a non-nil engine")
	}
}
