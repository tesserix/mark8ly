package storefront

import (
	"reflect"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/branding"
)

// TestPublicBrandingExposureIsExactlyWhatTheCommentClaims pins the boundary
// PublicBrandingResponse's doc comment describes.
//
// #694: that comment claimed custom_css was excluded while the struct
// included it. Nothing failed, because a comment cannot fail — and a reader
// auditing what the public branding endpoint exposes reads the comment, not
// the 36-field struct. This test makes the claim executable, so the next
// person who adds a field to StoreBranding has to decide, explicitly,
// whether it is public.
//
// Adding a field to StoreBranding without adding it here is the failure this
// catches. If you are here because it failed: decide whether the new field is
// public. If it is, add it to PublicBrandingResponse. If it is not, add it to
// withheld below AND to the doc comment.
func TestPublicBrandingExposureIsExactlyWhatTheCommentClaims(t *testing.T) {
	// Withheld from the storefront. Identifiers and timestamps, plus
	// ReturnPolicy, which only the admin DTO carries.
	withheld := map[string]struct{}{
		"ID":        {},
		"TenantID":  {},
		"StoreID":   {},
		"CreatedAt": {},
		"UpdatedAt": {},

		"ReturnPolicy": {},
	}

	public := map[string]struct{}{}
	pt := reflect.TypeOf(PublicBrandingResponse{})
	for i := 0; i < pt.NumField(); i++ {
		public[pt.Field(i).Name] = struct{}{}
	}

	mt := reflect.TypeOf(branding.StoreBranding{})
	for i := 0; i < mt.NumField(); i++ {
		name := mt.Field(i).Name
		_, isPublic := public[name]
		_, isWithheld := withheld[name]

		switch {
		case isPublic && isWithheld:
			t.Errorf("StoreBranding.%s is listed as withheld but IS on PublicBrandingResponse", name)
		case !isPublic && !isWithheld:
			t.Errorf(
				"StoreBranding.%s reaches neither PublicBrandingResponse nor the withheld list — "+
					"decide whether it is public and record the decision in both places",
				name,
			)
		}
	}

	// The specific regression: custom_css is public on purpose. If someone
	// makes it non-public they must change the doc comment too, and this
	// says so at the point of failure.
	if _, ok := public["CustomCSS"]; !ok {
		t.Error("CustomCSS is no longer on PublicBrandingResponse — update the doc comment, which states it IS carried")
	}
}
