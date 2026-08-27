package inbox_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/inbox"
)

func TestDeriveSeverity(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	ptr := func(d time.Duration) *time.Time { t := now.Add(d); return &t }

	cases := []struct {
		name  string
		dueAt *time.Time
		want  string
	}{
		{"no due date is normal", nil, inbox.SeverityNormal},
		{"far future is normal", ptr(72 * time.Hour), inbox.SeverityNormal},
		{"exactly 24h out is warning", ptr(24 * time.Hour), inbox.SeverityWarning},
		{"inside 24h is warning", ptr(time.Hour), inbox.SeverityWarning},
		{"exactly now is critical", ptr(0), inbox.SeverityCritical},
		{"past due is critical", ptr(-time.Minute), inbox.SeverityCritical},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, inbox.DeriveSeverity(tc.dueAt, now))
		})
	}
}
