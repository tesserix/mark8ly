package emailtemplates

// sender.go — the test-send dispatcher for the platform admin authoring
// surface.
//
// marketplace-api's equivalent (sendgrid_test_sender.go) is a bespoke,
// minimum-viable SendGrid v3 HTTP client, because marketplace-api's
// package had no existing Sender abstraction to reuse. platform-api
// already has one — internal/notification.Sender, with SendGrid, Resend
// and a fallback chain wired in cmd/server/main.go (notification.NewFromConfig)
// — so this file is an adapter onto that, not a second HTTP client.
//
// One consequence of reusing it: notification.NewFromConfig NEVER returns
// nil — with no provider key configured it returns a LogSender that prints
// to stdout, matching how every other platform-api email (password reset,
// login OTP, ...) already degrades in dev. A test-send through this
// adapter therefore does not answer 503 not_configured in that
// configuration the way marketplace-api's does; it "succeeds" by logging
// instead of emailing. That is an intentional, existing platform-api
// behaviour this adapter inherits rather than papers over — see
// TestSender's construction site in cmd/server/main.go.

import (
	"context"
	"errors"

	"github.com/mark8ly/platform-api/internal/notification"
)

// TestSender dispatches a rendered template to a single recipient for the
// admin authoring surface's test-send route.
type TestSender interface {
	SendTest(ctx context.Context, to string, r Rendered) error
}

// notificationTestSender adapts a notification.Sender + From address into
// a TestSender.
type notificationTestSender struct {
	sender notification.Sender
	from   string
}

// NewNotificationTestSender builds a TestSender backed by sender (the same
// notification.Sender instance cmd/server/main.go builds for real sends)
// and the service's configured From address.
func NewNotificationTestSender(sender notification.Sender, from string) TestSender {
	return &notificationTestSender{sender: sender, from: from}
}

func (s *notificationTestSender) SendTest(ctx context.Context, to string, r Rendered) error {
	if s == nil || s.sender == nil {
		return errors.New("emailtemplates: no notification sender configured")
	}
	return s.sender.Send(ctx, notification.Email{
		To:       to,
		From:     s.from,
		Subject:  r.Subject,
		HTMLBody: r.HTMLBody,
		TextBody: r.TextBody,
	})
}
