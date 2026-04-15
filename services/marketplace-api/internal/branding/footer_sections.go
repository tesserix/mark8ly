package branding

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"gorm.io/datatypes"

	apperrors "github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Validation caps. These mirror the admin UI's maxLength attrs — update BOTH when changing a cap.
const (
	maxFooterSections    = 6
	maxFooterItemsPerSec = 10
	maxFooterLabelLen    = 60
	maxFooterURLLen      = 500
	footerItemKindPage   = "page"
	footerItemKindURL    = "url"
)

var footerSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)

// IsSafeURL returns true if the URL uses an allowed scheme. Merchant
// authored URLs are restricted to http://, https://, and site-
// relative paths beginning with /. This blocks javascript: and
// data: URIs that would enable stored XSS if rendered verbatim in
// <a href> or <img src>.
func IsSafeURL(u string) bool {
	u = strings.TrimSpace(u)
	if u == "" {
		return false
	}
	return strings.HasPrefix(u, "http://") ||
		strings.HasPrefix(u, "https://") ||
		strings.HasPrefix(u, "/")
}

// ValidateFooterSections enforces the shape constraints for a
// merchant-authored footer. Returns an apperrors.ValidationFailed with
// field="footer_sections" on any violation.
func ValidateFooterSections(sections []FooterSection) error {
	if len(sections) > maxFooterSections {
		return apperrors.ValidationFailed("footer_sections",
			fmt.Sprintf("at most %d sections allowed", maxFooterSections))
	}
	for i, sec := range sections {
		label := strings.TrimSpace(sec.Label)
		if label == "" {
			return apperrors.ValidationFailed("footer_sections",
				fmt.Sprintf("section[%d].label is required", i))
		}
		if len(label) > maxFooterLabelLen {
			return apperrors.ValidationFailed("footer_sections",
				fmt.Sprintf("section[%d].label exceeds %d chars", i, maxFooterLabelLen))
		}
		if len(sec.Items) > maxFooterItemsPerSec {
			return apperrors.ValidationFailed("footer_sections",
				fmt.Sprintf("section[%d] has more than %d items", i, maxFooterItemsPerSec))
		}
		for j, item := range sec.Items {
			itemLabel := strings.TrimSpace(item.Label)
			if itemLabel == "" {
				return apperrors.ValidationFailed("footer_sections",
					fmt.Sprintf("section[%d].items[%d].label is required", i, j))
			}
			if len(itemLabel) > maxFooterLabelLen {
				return apperrors.ValidationFailed("footer_sections",
					fmt.Sprintf("section[%d].items[%d].label exceeds %d chars", i, j, maxFooterLabelLen))
			}
			switch item.Kind {
			case footerItemKindPage:
				if item.URL != "" {
					return apperrors.ValidationFailed("footer_sections",
						fmt.Sprintf("section[%d].items[%d] kind=page must not set url", i, j))
				}
				if !footerSlugPattern.MatchString(item.PageSlug) {
					return apperrors.ValidationFailed("footer_sections",
						fmt.Sprintf("section[%d].items[%d].page_slug is invalid", i, j))
				}
			case footerItemKindURL:
				if item.PageSlug != "" {
					return apperrors.ValidationFailed("footer_sections",
						fmt.Sprintf("section[%d].items[%d] kind=url must not set page_slug", i, j))
				}
				u := strings.TrimSpace(item.URL)
				if u == "" {
					return apperrors.ValidationFailed("footer_sections",
						fmt.Sprintf("section[%d].items[%d].url is required", i, j))
				}
				if len(u) > maxFooterURLLen {
					return apperrors.ValidationFailed("footer_sections",
						fmt.Sprintf("section[%d].items[%d].url exceeds %d chars", i, j, maxFooterURLLen))
				}
				if !IsSafeURL(u) {
					return apperrors.ValidationFailed("footer_sections",
						fmt.Sprintf("section[%d].items[%d].url must start with http://, https:// or /", i, j))
				}
			default:
				return apperrors.ValidationFailed("footer_sections",
					fmt.Sprintf("section[%d].items[%d].kind must be %q or %q", i, j, footerItemKindPage, footerItemKindURL))
			}
		}
	}
	return nil
}

// ParseFooterSections decodes the raw datatypes.JSON into domain types.
// Returns an empty slice for nil/empty input so callers can treat it as
// a non-nil value.
func ParseFooterSections(raw datatypes.JSON) ([]FooterSection, error) {
	if len(raw) == 0 {
		return []FooterSection{}, nil
	}
	var sections []FooterSection
	if err := json.Unmarshal(raw, &sections); err != nil {
		return nil, apperrors.ValidationFailed("footer_sections", "invalid JSON")
	}
	return sections, nil
}

// EncodeFooterSections marshals validated domain sections back to JSONB.
func EncodeFooterSections(sections []FooterSection) (datatypes.JSON, error) {
	b, err := json.Marshal(sections)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(b), nil
}
