package notification

import (
	"strings"
	"testing"
)

func TestSendgridFrom_SplitsDisplayName(t *testing.T) {
	got := sendgridFrom(`"Mark8ly" <noreply@mark8ly.com>`)
	if got.Email != "noreply@mark8ly.com" || got.Name != "Mark8ly" {
		t.Fatalf("got %+v", got)
	}
	if got := sendgridFrom("noreply@mark8ly.com"); got.Email != "noreply@mark8ly.com" || got.Name != "" {
		t.Fatalf("bare address: got %+v", got)
	}
}

func TestRenderInvitation_NoInviterNoArticle(t *testing.T) {
	msg, err := RenderInvitation("a@b.com", "noreply@mark8ly.com", InvitationVars{
		TenantName: "The Bondi Store", Role: "admin", AcceptURL: "https://x", ExpiresIn: "72 hours", SupportEmail: "hello@mark8ly.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{msg.HTMLBody, msg.TextBody} {
		if strings.Contains(body, "as a admin") || strings.Contains(body, "as a <strong") {
			t.Errorf("article before role survived:\n%s", body)
		}
		if !strings.Contains(body, "hello@mark8ly.com") {
			t.Errorf("support address missing")
		}
	}
	if !strings.Contains(msg.HTMLBody, "You&rsquo;ve been invited to join The Bondi Store") {
		t.Errorf("preheader without inviter should read naturally")
	}
}
