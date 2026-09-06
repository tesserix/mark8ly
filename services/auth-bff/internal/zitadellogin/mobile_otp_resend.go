package zitadellogin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
)

// ChallengeIssuer mints a fresh emailed code and delivers it. Narrow on
// purpose, exactly like CodeVerifier above it: this package needs the
// issue, not the whole OTP subsystem, and stating the dependency this way
// keeps the direction explicit.
//
// *loginotp.Gate satisfies this shape directly; cmd/server wires the SAME
// gate that backs CodeVerifier through an adapter so every surface issues
// codes identically and there is one rate-limit budget, not two.
type ChallengeIssuer interface {
	IssueChallenge(ctx context.Context, email, ip string) error
}

// ErrChallengeRateLimited is the ONE issuer failure this package must tell
// apart from the rest.
//
// The limiter allows a small number of codes per window (emailotp's
// DefaultMaxPerWindow within DefaultRateWindow), and the login itself
// spends one. A merchant who has spent the rest must be told to wait:
// folding this into a generic failure leaves them tapping Resend against a
// wall, which is precisely the dead end this feature exists to remove.
//
// Declared here rather than reusing emailotp's sentinel so zitadellogin
// keeps knowing nothing about the OTP subsystem; cmd/server's adapter
// translates emailotp.ErrRateLimited into this.
var ErrChallengeRateLimited = errors.New("zitadellogin: too many codes requested")

// WithChallengeIssuer enables the mobile resend route. Absent it the route
// refuses with 500 rather than half-working — a Resend button that
// silently does nothing is worse than no button.
//
// Separate from WithStepUp because the two dependencies are wired from the
// same gate but are not both required by the same routes: /otp/verify
// needs only the verifier.
func (h *Handler) WithChallengeIssuer(ci ChallengeIssuer) *Handler {
	h.challenges = ci
	return h
}

type mobileOTPResendRequest struct {
	PendingToken string `json:"pending_token"`
}

// mobileOTPResend mails a fresh code for a login that stopped at the
// email-OTP gate, and hands back a FRESH pending token to resume from.
//
// # Why it re-seals rather than only re-mailing
//
// The pending token and the emailed code expire on the same order of
// minutes. A resend that minted only a new code would hand the merchant a
// fresh code against an about-to-expire challenge: they would type a
// CORRECT code and be told it is wrong, which is worse than having no
// resend at all. Re-sealing restarts both windows together, so the client
// MUST swap to the returned token — the old one is not revoked (a sealed
// AES-GCM value cannot be), it is simply the stale half of the pair.
//
// The address is read from the SEALED token and never from the request
// body — the same binding mobileOTPVerify documents. Here it matters even
// more directly: trusting a body-supplied address would turn this route
// into a way to mail a code to an address of the caller's choosing.
func (h *Handler) mobileOTPResend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !internalAuthorized(r, h.internalAuthSecret) {
		writeUnauthorized(w)
		return
	}
	if h.challenges == nil || h.pending == nil {
		slog.ErrorContext(ctx, "zitadellogin: mobile otp resend reached with the challenge issuer not configured")
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	var req mobileOTPResendRequest
	if err := decodeJSON(r, &req); err != nil || req.PendingToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}

	pending, err := h.pending.OpenPendingLogin(req.PendingToken)
	if err != nil || pending == nil {
		// Forged and expired answer identically, as everywhere else here:
		// distinguishing them tells a prober which half to attack.
		slog.WarnContext(ctx, "zitadellogin: mobile otp resend rejected an unusable pending token", "err", err)
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_challenge"})
		return
	}
	if pending.Email == "" || pending.UID == "" {
		// A server-side inconsistency, not a user error: without an
		// address there is nothing to mail, and without a uid the re-seal
		// would be refused.
		slog.ErrorContext(ctx, "zitadellogin: pending token carried no identity; cannot resend")
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	if err := h.challenges.IssueChallenge(ctx, pending.Email, clientIPFromContext(ctx)); err != nil {
		if errors.Is(err, ErrChallengeRateLimited) {
			// Its own code, deliberately: "wait a few minutes" is the only
			// advice a merchant here can act on.
			slog.WarnContext(ctx, "zitadellogin: mobile otp resend rate limited", "user_id", pending.UID)
			writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate_limited"})
			return
		}
		slog.ErrorContext(ctx, "zitadellogin: could not issue a fresh code", "err", err, "user_id", pending.UID)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	// Same Zitadel session, same identity — only the sealed value's own
	// lifetime is reset. Re-resolving the subject from Zitadel here would
	// be a second round trip for a value that is already proven.
	token, err := h.mintPendingLogin(pending.UID, pending.Email, pending.TenantID,
		Session{ID: pending.ZitadelSessionID, Token: pending.ZitadelSessionToken})
	if err != nil {
		// The code IS in the merchant's inbox by now. Reporting a failure
		// they could read as "no code was sent" would send them round the
		// loop again, so this is logged loudly and answered as our fault.
		slog.ErrorContext(ctx, "zitadellogin: sent a fresh code but could not re-seal the pending token", "err", err, "user_id", pending.UID)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"sent": true, "pending_token": token},
	})
}
