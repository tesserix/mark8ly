package zitadellogin

import (
	"context"
	"log/slog"
	"net/http"
)

type mobileTOTPVerifyRequest struct {
	PendingToken string `json:"pending_token"`
	Code         string `json:"code"`
}

// mobileTOTPVerify completes a login that stopped at the TOTP gate on the
// mobile surface (mark8ly#686 item 2).
//
// # Why this exists beside mobileTOTP
//
// mobileTOTP (the /zitadel/mobile/totp route) is the WEB totp handler in
// token-issuing mode: it requires an auth_request_id and a
// workspace_tenant in the request body. A native client can supply
// neither — the login call mints the auth request server-side and never
// returns its id, and the tenant is resolved from the email inside
// marketplace-api, not on the device. So that route, while mounted, is
// unreachable from the app: a merchant with TOTP enrolled could not sign
// in at all.
//
// This handler is mobileOTPVerify's twin instead, and deliberately so.
// Both step-ups now hand the client ONE thing — an opaque pending_token —
// so "a step-up carries a pending_token" is simply true on mobile rather
// than something every client has to branch on. It also keeps the raw
// Zitadel session id/token off the device: a sealed value already exists
// for exactly this purpose.
//
// Like mobileOTPVerify it mints a FRESH auth request rather than carrying
// the login's own: the authorization code from the login call was
// discarded when the gauntlet demanded a step-up, and carrying it would
// make success depend on Zitadel's code lifetime outlasting the user
// reaching for their phone — so a CORRECT code entered a minute late
// would fail with nothing to explain it.
func (h *Handler) mobileTOTPVerify(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !internalAuthorized(r, h.internalAuthSecret) {
		writeUnauthorized(w)
		return
	}
	if h.pending == nil || h.tokens == nil {
		slog.ErrorContext(ctx, "zitadellogin: mobile totp verify reached with step-up not configured")
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	var req mobileTOTPVerifyRequest
	if err := decodeJSON(r, &req); err != nil || req.PendingToken == "" || req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}

	// Identity and tenant come from the sealed token and NOTHING else —
	// the same binding mobileOTPVerify states. A login_name or tenant in
	// the body is not merely ignored: there is no field to put one in.
	pending, err := h.pending.OpenPendingLogin(req.PendingToken)
	if err != nil || pending == nil {
		// Forged and expired answer identically: distinguishing them tells
		// a prober which half to attack.
		slog.WarnContext(ctx, "zitadellogin: mobile totp verify rejected an unusable pending token", "err", err)
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_challenge"})
		return
	}
	if pending.ZitadelSessionID == "" || pending.ZitadelSessionToken == "" {
		// A server-side inconsistency, not a user error. Reporting it as a
		// bad code would have a merchant retype a correct one forever.
		slog.ErrorContext(ctx, "zitadellogin: pending token carried no zitadel session; cannot resume totp", "user_id", pending.UID)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	sess, err := h.c.VerifyTOTP(ctx, Session{ID: pending.ZitadelSessionID, Token: pending.ZitadelSessionToken}, req.Code)
	if err != nil {
		// Shared with the web totp route so a rejected code cannot be
		// answered two different ways by two surfaces.
		h.respondTOTPVerifyError(ctx, w, err)
		return
	}

	authRequestID, err := h.newAuthRequest(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "zitadellogin: could not create an auth request to resume after totp", "err", err, "user_id", pending.UID)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	// respondOutcome, not respondTokens: unlike the email-OTP challenge —
	// which runs AFTER the gauntlet and therefore only has tokens left to
	// mint — the TOTP gate is reached BEFORE it. finishComplete re-resolves
	// the subject from Zitadel and runs the gauntlet here, which is also
	// what lets a login needing both step-ups chain into the email-OTP
	// challenge with a fresh pending token.
	res, err := h.c.CompleteAfterFactor(ctx, authRequestID, sess)
	h.respondOutcome(w, r, res, err, authRequestID, sess, pending.TenantID, true)
}

// mintTOTPPending seals the state /mobile/totp/verify will resume from.
//
// Unlike the email-OTP mint, this runs BEFORE finishComplete, so no
// subject has been resolved yet — hence the SessionFactors/UserEmail reads
// here. They are cheap and already proven: DecideSufficiency read the same
// factors moments ago to reach OutcomeFactorRequired at all.
//
// The uid and tenant are not decoration: SealPending REFUSES a pending
// value without both, precisely so a sealed challenge is always bound to
// an account.
func (h *Handler) mintTOTPPending(ctx context.Context, sess Session, workspaceTenant string) (string, error) {
	factors, err := h.c.SessionFactors(ctx, sess.ID)
	if err != nil {
		return "", err
	}
	if factors.UserID == "" {
		return "", errStepUpUnresolved
	}
	// From Zitadel's record of the session subject, never a request body —
	// the same rule finishComplete states, applied one step earlier.
	email, err := h.c.UserEmail(ctx, factors.UserID)
	if err != nil {
		return "", err
	}
	return h.mintPendingLogin(factors.UserID, email, workspaceTenant, sess)
}
