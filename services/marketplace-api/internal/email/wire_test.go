package email

import (
	"encoding/json"
	"strings"
	"testing"
)

// storeMsg is a customer-facing envelope with a full store sender
// identity — the shape #718 introduces.
func storeMsg() Message {
	return Message{
		From:     "nadias-ceramics@mail.mark8ly.com",
		FromName: "Nadia's Ceramics",
		ReplyTo:  "hello@nadiasceramics.com",
		To:       "buyer@example.com",
		ToName:   "A Buyer",
		Subject:  "Your order is confirmed",
		HTMLBody: "<p>hi</p>",
		TextBody: "hi",
		CustomArgs: map[string]string{
			"product":   "mark8ly",
			"kind":      "invoice",
			"tenant_id": "d3b07384-d9a0-4f1e-9b1a-000000000001",
		},
	}
}

// TestSendGridRequest_SenderIdentity pins every field of the From /
// Reply-To pair onto SendGrid's wire shape. FromName had two callers
// before #718 and no test proving it reached the provider at all.
func TestSendGridRequest_SenderIdentity(t *testing.T) {
	got := buildSendGridRequest(storeMsg())

	if got.From.Email != "nadias-ceramics@mail.mark8ly.com" {
		t.Errorf("from email = %q", got.From.Email)
	}
	if got.From.Name != "Nadia's Ceramics" {
		t.Errorf("from name = %q; FromName must reach the provider", got.From.Name)
	}
	if got.ReplyTo == nil {
		t.Fatal("reply_to omitted; a set ReplyTo must be emitted")
	}
	if got.ReplyTo.Email != "hello@nadiasceramics.com" {
		t.Errorf("reply_to = %q", got.ReplyTo.Email)
	}
}

// TestSendGridRequest_NoReplyTo_OmitsField guards the 400 SendGrid
// returns for an empty reply_to object: platform mail sets no ReplyTo,
// so the key must be absent from the JSON, not present-and-empty.
func TestSendGridRequest_NoReplyTo_OmitsField(t *testing.T) {
	msg := storeMsg()
	msg.ReplyTo = ""

	if got := buildSendGridRequest(msg); got.ReplyTo != nil {
		t.Fatalf("reply_to = %+v, want nil", got.ReplyTo)
	}
	raw, err := json.Marshal(buildSendGridRequest(msg))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "reply_to") {
		t.Errorf("reply_to present in payload: %s", raw)
	}
}

// TestSendGridRequest_CustomArgsSurvive is the #718 acceptance check
// that bounce/complaint attribution still reaches provider webhooks
// after the sender identity change.
func TestSendGridRequest_CustomArgsSurvive(t *testing.T) {
	got := buildSendGridRequest(storeMsg())

	for k, want := range map[string]string{
		"product":   "mark8ly",
		"kind":      "invoice",
		"tenant_id": "d3b07384-d9a0-4f1e-9b1a-000000000001",
	} {
		if got.CustomArgs[k] != want {
			t.Errorf("custom_args[%q] = %q, want %q", k, got.CustomArgs[k], want)
		}
	}
}

// TestResendRequest_SenderIdentity pins Resend's inline RFC-5322 From
// and its string-form reply_to.
func TestResendRequest_SenderIdentity(t *testing.T) {
	got := buildResendRequest(storeMsg())

	const wantFrom = "Nadia's Ceramics <nadias-ceramics@mail.mark8ly.com>"
	if got.From != wantFrom {
		t.Errorf("from = %q, want %q", got.From, wantFrom)
	}
	if got.ReplyTo != "hello@nadiasceramics.com" {
		t.Errorf("reply_to = %q", got.ReplyTo)
	}
}

// TestResendRequest_NoFromName_BareAddress keeps the pre-#718 behaviour
// for mail that sets no display name.
func TestResendRequest_NoFromName_BareAddress(t *testing.T) {
	msg := storeMsg()
	msg.FromName = ""
	msg.ReplyTo = ""

	got := buildResendRequest(msg)
	if got.From != "nadias-ceramics@mail.mark8ly.com" {
		t.Errorf("from = %q, want the bare address", got.From)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "reply_to") {
		t.Errorf("reply_to present in payload: %s", raw)
	}
}

// TestResendRequest_TagsCarryAttribution mirrors the SendGrid
// custom_args check — Resend echoes tags back on webhook events.
func TestResendRequest_TagsCarryAttribution(t *testing.T) {
	got := buildResendRequest(storeMsg())

	seen := map[string]string{}
	for _, tag := range got.Tags {
		seen[tag.Name] = tag.Value
	}
	if seen["product"] != "mark8ly" || seen["kind"] != "invoice" {
		t.Errorf("tags = %+v", got.Tags)
	}
	if seen["tenant_id"] != "d3b07384-d9a0-4f1e-9b1a-000000000001" {
		t.Errorf("tenant_id tag = %q", seen["tenant_id"])
	}
}

// TestValidate_ReplyTo refuses a Reply-To that would strand every reply.
// A broken value must fail the send rather than ship a dead header.
func TestValidate_ReplyTo(t *testing.T) {
	cases := []struct {
		name    string
		replyTo string
		wantErr bool
	}{
		{"unset is fine", "", false},
		{"real address", "hello@nadiasceramics.com", false},
		{"no at sign", "hello.nadiasceramics.com", true},
		{"no domain", "hello@", true},
		{"unroutable .local", "billing@mark8ly.local", true},
		{"bare hostname", "hello@localhost", true},
		{"header injection", "a@b.com\r\nBcc: victim@example.com", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg := storeMsg()
			msg.ReplyTo = c.replyTo
			err := validate(msg)
			if c.wantErr && err == nil {
				t.Fatalf("validate(reply-to %q) = nil, want error", c.replyTo)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("validate(reply-to %q) = %v, want nil", c.replyTo, err)
			}
		})
	}
}

// TestValidate_RejectsHeaderInjection covers the boundary no call site
// can bypass. The single-@ Reply-To case matters most: ValidateRecipient
// alone lets it through, because it only rejects a SECOND "@" in the
// domain — so "a@b.com\r\nBcc: x" is caught here and nowhere else.
func TestValidate_RejectsHeaderInjection(t *testing.T) {
	cases := []struct {
		field  string
		mutate func(*Message)
	}{
		{"FromName", func(m *Message) { m.FromName = "Nadia's\r\nBcc: victim@example.com" }},
		{"From", func(m *Message) { m.From = "a@b.com\nX-Spoof: 1" }},
		{"To", func(m *Message) { m.To = "a@b.com\r\nCc: victim@example.com" }},
		{"ToName", func(m *Message) { m.ToName = "Buyer\r\nSubject: spoofed" }},
		{"ReplyTo", func(m *Message) { m.ReplyTo = "a@b.com\r\nBcc: victim@example.com" }},
		{"Subject", func(m *Message) { m.Subject = "Hi\r\nBcc: victim@example.com" }},
		{"NUL in FromName", func(m *Message) { m.FromName = "Nadia\x00" }},
	}
	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			msg := storeMsg()
			c.mutate(&msg)
			if err := validate(msg); err == nil {
				t.Fatalf("validate accepted an injected %s", c.field)
			}
		})
	}
}
