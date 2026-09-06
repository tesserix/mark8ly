package authbffclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// ErrRateLimited is auth-bff refusing to mail another code because the
// address has spent its budget for the window.
//
// Kept distinct from every other failure on purpose: it is the only one
// whose remedy is "wait", and flattening it into a generic error leaves a
// merchant tapping Resend against a wall with copy that suggests retrying
// immediately will help.
var ErrRateLimited = errors.New("authbffclient: too many codes requested")

// ResendResult is what a successful resend hands back.
type ResendResult struct {
	// PendingToken is a FRESH sealed challenge. It is not optional: the
	// emailed code and the pending token expire on the same order of
	// minutes, so a resend that did not restart both would give the
	// merchant a correct code against a dead challenge. The caller MUST
	// replace the token it holds with this one.
	PendingToken string
}

type resendWire struct {
	Data *struct {
		Sent         bool   `json:"sent"`
		PendingToken string `json:"pending_token"`
	} `json:"data"`
}

// ResendOTP asks auth-bff to mail a fresh code for an outstanding
// email-OTP challenge.
//
// It posts to the MOBILE route, not auth-bff's /auth/otp/resend: that one
// resumes from the pending cookie the browser holds, which a native client
// does not have. Everything identifying travels inside the sealed token,
// for the same reason VerifyOTP sends no email — a body-supplied address
// would make this a way to mail a code anywhere.
func (c *MobileLoginClient) ResendOTP(ctx context.Context, pendingToken string) (ResendResult, error) {
	b, err := json.Marshal(map[string]any{"pending_token": pendingToken})
	if err != nil {
		return ResendResult{}, fmt.Errorf("authbffclient: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/auth/zitadel/mobile/otp/resend", bytes.NewReader(b))
	if err != nil {
		return ResendResult{}, fmt.Errorf("authbffclient: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		req.Header.Set("X-Internal-Auth", c.secret)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return ResendResult{}, fmt.Errorf("authbffclient: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return ResendResult{}, ErrRateLimited
	case resp.StatusCode == http.StatusUnauthorized:
		// The challenge is forged or expired. Same sentinel the verify
		// path uses, so the handler answers both the same way.
		return ResendResult{}, ErrInvalidCredentials
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return ResendResult{}, fmt.Errorf("authbffclient: auth-bff returned %d: %s", resp.StatusCode, string(raw))
	}

	var wire resendWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return ResendResult{}, fmt.Errorf("authbffclient: decode resend response: %w", err)
	}
	if wire.Data == nil || wire.Data.PendingToken == "" {
		// A 200 with no fresh token would leave the client verifying
		// against the old challenge — a correct code failing for reasons
		// nothing on screen could explain. Louder here than there.
		return ResendResult{}, errors.New("authbffclient: resend returned no pending token")
	}
	return ResendResult{PendingToken: wire.Data.PendingToken}, nil
}
