package zitadellogin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
)

// PendingLogin is the state a mobile step-up must carry between the login
// call and the challenge that completes it.
//
// It mirrors session.Pending — the browser's encrypted pending cookie —
// deliberately: one concept, two deliveries. A native client has no
// cookie, so it receives the same payload sealed into an opaque value it
// hands back verbatim.
type PendingLogin struct {
	UID      string
	Email    string
	TenantID string
	// The Zitadel session that already satisfied the password. The
	// challenge re-finalizes it against a fresh auth request to obtain an
	// authorization code, because the code from the login call was
	// discarded when the gauntlet demanded a step-up.
	//
	// The alternative — carrying that discarded code — would make success
	// depend on Zitadel's authorization-code lifetime outlasting the user
	// fetching an email, so a CORRECT code entered a minute late would
	// fail with nothing to explain it.
	ZitadelSessionID    string
	ZitadelSessionToken string
}

// CodeVerifier checks an emailed one-time code for an address. Narrow on
// purpose: zitadellogin needs the check, not the whole OTP subsystem, and
// stating the dependency this way keeps the direction explicit.
type CodeVerifier interface {
	VerifyCode(ctx context.Context, email, code string) error
}

// PendingStore seals and opens the step-up state. Backed by the session
// manager's AES-GCM, so the mobile token and the pending cookie share one
// format and one key.
type PendingStore interface {
	SealPendingLogin(p PendingLogin) (string, error)
	OpenPendingLogin(value string) (*PendingLogin, error)
}

// WithStepUp enables the mobile email-OTP challenge. Absent either
// dependency the route refuses rather than half-working.
func (h *Handler) WithStepUp(cv CodeVerifier, ps PendingStore) *Handler {
	h.codes = cv
	h.pending = ps
	return h
}

type mobileOTPRequest struct {
	PendingToken string `json:"pending_token"`
	Code         string `json:"code"`
}

// mobileOTPVerify completes a login that stopped at the email-OTP gate.
//
// A fresh install is ALWAYS an unrecognised device, so this is the common
// first sign-in on mobile, not an edge case.
func (h *Handler) mobileOTPVerify(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if !internalAuthorized(r, h.internalAuthSecret) {
		writeUnauthorized(w)
		return
	}
	if h.codes == nil || h.pending == nil || h.tokens == nil {
		slog.ErrorContext(ctx, "zitadellogin: mobile otp verify reached with step-up not configured")
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	var req mobileOTPRequest
	if err := decodeJSON(r, &req); err != nil || req.PendingToken == "" || req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}

	// Identity comes from the sealed token and NOTHING else. Any email in
	// the body is ignored — that binding is what stops a caller holding a
	// valid code of their own from completing another account's login by
	// naming a different address. Same rule the cookie path states.
	pending, err := h.pending.OpenPendingLogin(req.PendingToken)
	if err != nil || pending == nil {
		// Forged and expired are answered identically: distinguishing them
		// tells a prober which half to attack.
		slog.WarnContext(ctx, "zitadellogin: mobile otp verify rejected an unusable pending token", "err", err)
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_challenge"})
		return
	}

	if err := h.codes.VerifyCode(ctx, pending.Email, req.Code); err != nil {
		slog.WarnContext(ctx, "zitadellogin: mobile otp verify rejected a code", "user_id", pending.UID)
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_code"})
		return
	}

	// From here the user IS authenticated; anything that fails is our fault
	// and must not be reported as a credential problem, or they will retry
	// a correct code forever.
	if pending.ZitadelSessionID == "" || pending.ZitadelSessionToken == "" {
		slog.ErrorContext(ctx, "zitadellogin: pending token carried no zitadel session; cannot resume", "user_id", pending.UID)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	authRequestID, err := h.newAuthRequest(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "zitadellogin: could not create an auth request to resume after otp", "err", err, "user_id", pending.UID)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	sess := Session{ID: pending.ZitadelSessionID, Token: pending.ZitadelSessionToken}
	res, err := h.c.CompleteIfSufficient(ctx, authRequestID, sess, false)
	if err != nil || res.Outcome != OutcomeComplete {
		// The session satisfied Zitadel moments ago at login; anything else
		// now means it lapsed or policy changed mid-flow.
		slog.ErrorContext(ctx, "zitadellogin: could not resume the session after otp",
			"err", err, "outcome", res.Outcome, "user_id", pending.UID)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	h.respondTokens(w, r, res, pending.UID, pending.Email, pending.TenantID)
}

// errStepUpUnconfigured is returned by mintPendingLogin when the mobile
// step-up has no store wired, so login can refuse rather than answer with
// a challenge the client could never complete.
var errStepUpUnconfigured = errors.New("zitadellogin: step-up store not configured")

// errStepUpUnresolved is returned when the session a TOTP challenge would
// be sealed against has no readable subject. Sealing without one is not an
// option: the pending value REQUIRES a uid, which is what binds a
// challenge to an account.
var errStepUpUnresolved = errors.New("zitadellogin: could not resolve the session subject for a step-up")

// mintPendingLogin seals the state the OTP challenge will resume from.
func (h *Handler) mintPendingLogin(uid, email, tenantID string, sess Session) (string, error) {
	if h.pending == nil {
		return "", errStepUpUnconfigured
	}
	return h.pending.SealPendingLogin(PendingLogin{
		UID: uid, Email: email, TenantID: tenantID,
		ZitadelSessionID: sess.ID, ZitadelSessionToken: sess.Token,
	})
}
