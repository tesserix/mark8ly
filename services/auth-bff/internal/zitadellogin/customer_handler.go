// This file implements the STOREFRONT CUSTOMER login path. It looks
// unfinished compared to Handler (handler.go): it mints no session, sets no
// cookie, and calls nothing in internal/autologin. That is deliberate — see
// spec D11 (docs/superpowers/specs/2026-09-03-zitadel-migration-design.md).
//
// D11 supersedes an earlier framing (D10) that said the customer path would
// "skip the membership check". Reading the code made a simpler and safer
// shape available: rather than reuse the merchant gauntlet minus a check,
// this endpoint verifies the credential against Zitadel and returns
// {uid, email}. Full stop.
//
// Why not "finish" it by minting a session here too:
//
//   - Storefront customers are deliberately not OpenFGA members. Running them
//     through the merchant gauntlet would mean either adding a bypass flag to
//     that gauntlet (a second, weaker path baked into the trusted one) or
//     giving customers FGA tuples they have no reason to hold.
//   - The storefront mints its own `mp_customer_session` cookie, in its own
//     HMAC format, scoped to the exact request host — a customer signed in on
//     one store's subdomain must never be handed a session usable on another
//     store. Minting a cookie here would either introduce a THIRD session
//     format (alongside `m8_session` and `mp_customer_session`) or require
//     this package to know the request host of a storefront it has no
//     business knowing about. Either destroys the per-store isolation the
//     storefront's own minting code already gives for free.
//
// So: this handler verifies and returns. The storefront's existing sign-in
// action (apps/storefront/app/sign-in/actions.ts) keeps doing everything
// else it does today — resolving the host, resolving the store, minting the
// cookie, driving profile/loyalty side effects. Only the "verify the
// credential" step moves to Zitadel.
package zitadellogin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// CustomerHandler is the HTTP layer over Client for storefront customers. It
// shares Client and the sufficiency decision with Handler, but never touches
// CompleteFunc, session cookies, or internal/autologin — see the file
// comment above.
type CustomerHandler struct {
	c *Client

	// hostedLoginBaseURL mirrors Handler.hostedLoginBaseURL: the Zitadel
	// instance's own login UI, used ONLY as the OutcomeHandoff target for
	// factors this endpoint cannot collect. Optional: set via
	// WithHostedLoginBaseURL.
	hostedLoginBaseURL string
}

// NewCustomerHandler constructs a CustomerHandler.
func NewCustomerHandler(c *Client) *CustomerHandler {
	return &CustomerHandler{c: c}
}

// WithHostedLoginBaseURL sets the Zitadel instance base URL used to build the
// OutcomeHandoff redirect target. Mirrors Handler.WithHostedLoginBaseURL.
func (h *CustomerHandler) WithHostedLoginBaseURL(baseURL string) *CustomerHandler {
	h.hostedLoginBaseURL = strings.TrimSuffix(baseURL, "/")
	return h
}

// Register mounts the customer login routes onto the given gin.RouterGroup.
// Like Handler.Register, the handlers are plain net/http funcs; gin is only
// used to route, matching this package's existing style.
func (h *CustomerHandler) Register(r *gin.RouterGroup) {
	r.POST("/customer/login", func(c *gin.Context) {
		h.login(c.Writer, c.Request)
	})
	r.POST("/customer/totp", func(c *gin.Context) {
		h.totp(c.Writer, c.Request)
	})
}

type customerLoginRequest struct {
	AuthRequestID string `json:"auth_request_id"`
	LoginName     string `json:"login_name"`
	Password      string `json:"password"`
}

type customerTOTPRequest struct {
	AuthRequestID string `json:"auth_request_id"`
	SessionID     string `json:"session_id"`
	SessionToken  string `json:"session_token"`
	Code          string `json:"code"`
}

// login reads {auth_request_id, login_name, password}, creates a Zitadel
// password session, and asks sufficiency.go whether that session may
// finalize. It never mints a session or sets a cookie — see the file
// comment.
func (h *CustomerHandler) login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req customerLoginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	if req.AuthRequestID == "" || req.LoginName == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}

	// A wrong username and a wrong password take the same code path in
	// CreatePasswordSession and must take the same one here: a different
	// answer for "no such user" is an account-enumeration oracle on a public
	// storefront that anyone can probe.
	sess, err := h.c.CreatePasswordSession(ctx, req.LoginName, req.Password)
	if err != nil {
		h.respondSessionCreateError(ctx, w, err)
		return
	}

	// Password login through this endpoint is never a federated
	// (Google/Apple) identity — those never present a password to us at all.
	res, err := h.c.CompleteIfSufficient(ctx, req.AuthRequestID, sess, false)
	h.respondOutcome(ctx, w, res, err, req.AuthRequestID, sess)
}

// totp reads {auth_request_id, session_id, session_token, code}, submits the
// TOTP code against the session opened by login, and re-asks sufficiency.go
// whether the session may now finalize.
func (h *CustomerHandler) totp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req customerTOTPRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	if req.AuthRequestID == "" || req.SessionID == "" || req.SessionToken == "" || req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}

	sess, err := h.c.VerifyTOTP(ctx, Session{ID: req.SessionID, Token: req.SessionToken}, req.Code)
	if err != nil {
		h.respondTOTPVerifyError(ctx, w, err)
		return
	}

	res, err := h.c.CompleteAfterFactor(ctx, req.AuthRequestID, sess)
	h.respondOutcome(ctx, w, res, err, req.AuthRequestID, sess)
}

// respondOutcome is shared by login and totp: both end at
// CompleteIfSufficient / CompleteAfterFactor and switch on the same three
// outcomes as Handler.respondOutcome, but OutcomeComplete here resolves and
// returns an identity instead of running the post-identity gauntlet.
func (h *CustomerHandler) respondOutcome(
	ctx context.Context,
	w http.ResponseWriter,
	res Result,
	resErr error,
	authRequestID string,
	sess Session,
) {
	switch res.Outcome {
	case OutcomeComplete:
		if resErr != nil {
			// Not reachable per CompleteIfSufficient/CompleteAfterFactor's
			// contract, but refuse to report success on an outcome/error
			// mismatch rather than trust an incoherent result.
			slog.ErrorContext(ctx, "zitadellogin(customer): OutcomeComplete carried a non-nil error, refusing to complete", "err", resErr)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
			return
		}
		h.finishComplete(ctx, w, sess)

	case OutcomeFactorRequired:
		// No session minted here — this IS the MFA gate.
		writeJSON(w, http.StatusOK, map[string]any{
			"totp_required": true,
			"session_id":    sess.ID,
			"session_token": sess.Token,
		})

	default: // OutcomeHandoff, including the zero value.
		if resErr != nil {
			slog.ErrorContext(ctx, "zitadellogin(customer): handoff after a failed finalize (positive decision, exchange failed)",
				"err", resErr, "auth_request_id", authRequestID)
		} else {
			slog.InfoContext(ctx, "zitadellogin(customer): handoff (uncollectible factor or unreadable policy/session)",
				"auth_request_id", authRequestID)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"handoff_url":     h.handoffURL(authRequestID),
			"auth_request_id": authRequestID,
		})
	}
}

// finishComplete resolves the subject of the now-sufficient session and
// returns {uid, email}. It does not mint a session, set a cookie, or call
// anything in internal/autologin — see the file comment.
func (h *CustomerHandler) finishComplete(ctx context.Context, w http.ResponseWriter, sess Session) {
	// Result carries no subject — re-read it.
	factors, err := h.c.SessionFactors(ctx, sess.ID)
	if err != nil || factors.UserID == "" {
		slog.ErrorContext(ctx, "zitadellogin(customer): could not resolve session subject after a sufficient decision", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	// The email MUST come from Zitadel's own record of this session's
	// subject, never from a request body — the same defect fixed on the
	// merchant path in phase 2. A caller with valid credentials of their own
	// could otherwise submit an arbitrary login_name and walk away with an
	// identity response addressed to a victim's email of their choosing.
	email, err := h.c.UserEmail(ctx, factors.UserID)
	if err != nil {
		slog.ErrorContext(ctx, "zitadellogin(customer): could not resolve the verified email for session subject", "err", err, "user_id", factors.UserID)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"uid":   factors.UserID,
			"email": email,
		},
	})
}

// respondSessionCreateError maps CreatePasswordSession's errors.
//
// ErrBadCredentials and ErrUserNotFound MUST produce the identical response —
// collapsing them is the entire point of this function, and it matters more
// here than on the merchant path: this endpoint is reachable by anyone on a
// public storefront, so a different answer for "no such user" is a live
// account-enumeration oracle. Which one actually happened is logged for
// operators, never returned to the caller.
func (h *CustomerHandler) respondSessionCreateError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrBadCredentials):
		slog.WarnContext(ctx, "zitadellogin(customer): login rejected: bad credentials")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_credentials"})
	case errors.Is(err, ErrUserNotFound):
		slog.WarnContext(ctx, "zitadellogin(customer): login rejected: user not found")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_credentials"})
	case errors.Is(err, ErrUnavailable):
		slog.ErrorContext(ctx, "zitadellogin(customer): zitadel unavailable creating session", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "zitadel_unavailable"})
	default:
		slog.ErrorContext(ctx, "zitadellogin(customer): unexpected error creating session", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
	}
}

// respondTOTPVerifyError maps VerifyTOTP's errors. Unlike the login step,
// there is no enumeration concern here — the account is already established
// — but the wrong-code case still must never echo Zitadel's error body.
func (h *CustomerHandler) respondTOTPVerifyError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrBadCredentials):
		slog.WarnContext(ctx, "zitadellogin(customer): totp rejected: bad code")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_totp"})
	case errors.Is(err, ErrUserNotFound):
		// The session itself vanished/expired between steps.
		slog.WarnContext(ctx, "zitadellogin(customer): totp rejected: session not found")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_totp"})
	case errors.Is(err, ErrUnavailable):
		slog.ErrorContext(ctx, "zitadellogin(customer): zitadel unavailable verifying totp", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "zitadel_unavailable"})
	default:
		slog.ErrorContext(ctx, "zitadellogin(customer): unexpected error verifying totp", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
	}
}

// handoffURL builds the Aurora-branded hosted login's continuation URL for an
// auth request this endpoint decided it cannot (or should not) finish
// itself. Mirrors Handler.handoffURL. Returns "" when no hosted login base
// URL was configured; the caller still gets auth_request_id back.
func (h *CustomerHandler) handoffURL(authRequestID string) string {
	if h.hostedLoginBaseURL == "" {
		return ""
	}
	return fmt.Sprintf("%s/ui/v2/login/login?authRequestID=%s", h.hostedLoginBaseURL, url.QueryEscape(authRequestID))
}
