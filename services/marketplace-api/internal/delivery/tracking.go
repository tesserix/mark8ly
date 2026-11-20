package delivery

import (
	"errors"
	"net/url"
	"strings"
)

const numberToken = "{number}"

// ValidateTemplate checks a courier tracking-URL template: https, exactly one
// {number}, at most 500 characters.
func ValidateTemplate(t string) error {
	switch {
	case len(t) > 500:
		return errors.New("tracking URL must be at most 500 characters")
	case !strings.HasPrefix(t, "https://"):
		return errors.New("tracking URL must start with https://")
	case strings.Count(t, numberToken) != 1:
		return errors.New("tracking URL must contain {number} exactly once")
	}
	return nil
}

// BuildTrackingURL fills the template with the escaped tracking number.
// Returns false when there is nothing safe to link to.
func BuildTrackingURL(template, number string) (string, bool) {
	number = strings.TrimSpace(number)
	if template == "" || number == "" {
		return "", false
	}
	u := strings.ReplaceAll(template, numberToken, url.QueryEscape(number))
	if !strings.HasPrefix(u, "https://") {
		return "", false
	}
	return u, true
}
