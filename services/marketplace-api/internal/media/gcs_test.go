package media_test

import (
	"strings"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/media"
)

func TestBuildStorageKey_BasicFormat(t *testing.T) {
	got := media.BuildStorageKey("t-123", "abc123", "photo.jpg")
	want := "tenants/t-123/products/media/abc123/photo.jpg"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestBuildStorageKey_SanitizesFilename(t *testing.T) {
	got := media.BuildStorageKey("t1", "h", "Men's T-Shirt!.jpeg")
	want := "tenants/t1/products/media/h/Men-s-T-Shirt-.jpeg"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestBuildStorageKey_StripsLeadingSlash(t *testing.T) {
	got := media.BuildStorageKey("t1", "h", "/evil")
	if strings.Contains(got, "//") {
		t.Errorf("unexpected double slash: %q", got)
	}
	if !strings.HasSuffix(got, "/evil") {
		t.Errorf("unexpected suffix: %q", got)
	}
}

func TestBuildStorageKey_EmptyFilename_FallsBackToFile(t *testing.T) {
	got := media.BuildStorageKey("t1", "h", "")
	if !strings.HasSuffix(got, "/file") {
		t.Errorf("expected /file suffix: %q", got)
	}
}

func TestGCSUploader_SatisfiesUploader_And_SignedURLGenerator(t *testing.T) {
	// Compile-time assertions live in gcs.go. Runtime sanity: an
	// instance constructed from a nil-safe path would panic on use,
	// so we just assert the type system here via a typed nil.
	var u *media.GCSUploader
	var _ media.Uploader = u
	var _ media.SignedURLGenerator = u
}

func TestFakeUploader_DoesNotSatisfy_SignedURLGenerator(t *testing.T) {
	var u media.Uploader = media.NewFakeUploader()
	if _, ok := u.(media.SignedURLGenerator); ok {
		t.Error("FakeUploader must not implement SignedURLGenerator")
	}
}
