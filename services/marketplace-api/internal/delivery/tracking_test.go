package delivery

import (
	"strings"
	"testing"
)

func TestBuildTrackingURL(t *testing.T) {
	got, ok := BuildTrackingURL("https://c.in/t?awb={number}", "AB 12&#")
	if !ok || got != "https://c.in/t?awb=AB+12%26%23" {
		t.Fatalf("got %q %v", got, ok)
	}
	for _, c := range [][2]string{{"http://c.in/{number}", "1"}, {"", "1"}, {"https://c.in/{number}", "  "}} {
		if u, ok := BuildTrackingURL(c[0], c[1]); ok {
			t.Fatalf("%v should fail, got %q", c, u)
		}
	}
}

func TestValidateTemplate(t *testing.T) {
	if err := ValidateTemplate("https://c.in/t?awb={number}"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"https://c.in/", "https://c.in/{number}/{number}", "javascript:alert({number})", "https://c.in/{number}" + strings.Repeat("a", 500)} {
		if ValidateTemplate(bad) == nil {
			t.Fatalf("%q should be rejected", bad)
		}
	}
}
