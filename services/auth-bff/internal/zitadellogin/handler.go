package zitadellogin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// maxRequestBodyBytes bounds request decoding, matching the defensive limits
// already used for Zitadel's own responses in client.go.
const maxRequestBodyBytes = 8 * 1024

// CompleteFunc runs the shared post-identity gauntlet — FGA membership, the
// MFA gate, deviceguard, the email-OTP step-up, session minting. It is
// injected rather than imported so this package stays a Zitadel client and
// knows nothing about autologin.
type CompleteFunc func(ctx context.Context, w http.ResponseWriter, uid, email, tenantID string) error

// Handler is the HTTP layer over Client: it owns request/response shapes and
// the outcome switch, and defers every credential and sufficiency decision to
// the client and sufficiency packages respectively.
type Handler struct {
	c        *Client
	complete CompleteFunc

	// hostedLoginBaseURL is the Zitadel instance's own login UI (the
	// Aurora-branded hosted login), used ONLY as the OutcomeHandoff target
	// for factors this page cannot collect (passkeys, U2F, SMS OTP,
	// recovery codes). Optional: set via WithHostedLoginBaseURL. When
	// unset, a handoff still answers 200 with an empty handoff_url and the
	// auth_request_id, and logs a warning — the auth request itself is not
	// lost, only the convenience redirect.
	hostedLoginBaseURL string
}

// NewHandler constructs a Handler. complete may be nil in tests that never
// exercise OutcomeComplete.
func NewHandler(c *Client, complete CompleteFunc) *Handler {
	return &Handler{c: c, complete: complete}
}

// WithHostedLoginBaseURL sets the Zitadel instance base URL used to build the
// OutcomeHandoff redirect target. Mirrors the optional-config builder idiom
// used elsewhere in this service (e.g. session.Handler.WithRegistry).
func (h *Handler) WithHostedLoginBaseURL(baseURL string) *Handler {
	h.hostedLoginBaseURL = strings.TrimSuffix(baseURL, "/")
	return h
}

// Register mounts the Zitadel login routes onto the given gin.RouterGroup.
// The handlers are plain net/http (gin.WrapF) rather than gin.HandlerFunc:
// this package has no dependency on gin beyond mounting, and stays testable
// with httptest alone.
func (h *Handler) Register(r *gin.RouterGroup) {
	r.POST("/zitadel/login", gin.WrapF(h.login))
	r.POST("/zitadel/totp", gin.WrapF(h.totp))
}

type loginRequest struct {
	AuthRequestID   string `json:"auth_request_id"`
	LoginName       string `json:"login_name"`
	Password        string `json:"password"`
	WorkspaceTenant string `json:"workspace_tenant"`
}

type totpRequest struct {
	AuthRequestID   string `json:"auth_request_id"`
	LoginName       string `json:"login_name"`
	SessionID       string `json:"session_id"`
	SessionToken    string `json:"session_token"`
	Code            string `json:"code"`
	WorkspaceTenant string `json:"workspace_tenant"`
}

// login reads {auth_request_id, login_name, password, workspace_tenant},
// creates a Zitadel password session, and asks sufficiency.go whether that
// session may finalize.
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	if req.AuthRequestID == "" || req.LoginName == "" || req.Password == "" || req.WorkspaceTenant == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}

	// A wrong username and a wrong password take the same code path in
	// CreatePasswordSession and must take the same one here: a different
	// answer for "no such user" is an account-enumeration oracle.
	sess, err := h.c.CreatePasswordSession(ctx, req.LoginName, req.Password)
	if err != nil {
		h.respondSessionCreateError(ctx, w, err)
		return
	}

	// Password login through this page is never a federated (Google/Apple)
	// identity — those never present a password to us at all.
	res, err := h.c.CompleteIfSufficient(ctx, req.AuthRequestID, sess, false)
	h.respondOutcome(ctx, w, res, err, req.AuthRequestID, sess, req.LoginName, req.WorkspaceTenant)
}

// totp reads {auth_request_id, login_name, session_id, session_token, code,
// workspace_tenant}, submits the TOTP code against the session opened by
// login, and re-asks sufficiency.go whether the session may now finalize.
func (h *Handler) totp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req totpRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}
	if req.AuthRequestID == "" || req.LoginName == "" || req.SessionID == "" || req.SessionToken == "" ||
		req.Code == "" || req.WorkspaceTenant == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_request"})
		return
	}

	sess, err := h.c.VerifyTOTP(ctx, Session{ID: req.SessionID, Token: req.SessionToken}, req.Code)
	if err != nil {
		h.respondTOTPVerifyError(ctx, w, err)
		return
	}

	res, err := h.c.CompleteAfterFactor(ctx, req.AuthRequestID, sess)
	h.respondOutcome(ctx, w, res, err, req.AuthRequestID, sess, req.LoginName, req.WorkspaceTenant)
}

// respondOutcome is shared by login and totp: both end at CompleteIfSufficient
// / CompleteAfterFactor and switch on the same three outcomes.
func (h *Handler) respondOutcome(
	ctx context.Context,
	w http.ResponseWriter,
	res Result,
	resErr error,
	authRequestID string,
	sess Session,
	loginName string,
	workspaceTenant string,
) {
	switch res.Outcome {
	case OutcomeComplete:
		if resErr != nil {
			// Not reachable per CompleteIfSufficient/CompleteAfterFactor's
			// contract — OutcomeComplete is only ever returned with a nil
			// error — but refuse to complete a login on an outcome/error
			// mismatch rather than trust an incoherent result.
			slog.ErrorContext(ctx, "zitadellogin: OutcomeComplete carried a non-nil error, refusing to complete", "err", resErr)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
			return
		}
		h.finishComplete(ctx, w, res, sess, loginName, workspaceTenant)

	case OutcomeFactorRequired:
		// No session minted here — this IS the MFA gate. Minting now would
		// defeat the entire reason this package exists.
		writeJSON(w, http.StatusOK, map[string]any{
			"totp_required": true,
			"session_id":    sess.ID,
			"session_token": sess.Token,
		})

	default: // OutcomeHandoff, including the zero value.
		if resErr != nil {
			// finalize() ran, Zitadel returned a positive decision, and the
			// exchange itself failed — a real error, not routine
			// uncertainty. Distinguished from the nil-error case below so
			// an operator can tell "policy was unreadable" apart from
			// "we decided to finalize and it broke".
			slog.ErrorContext(ctx, "zitadellogin: handoff after a failed finalize (positive decision, exchange failed)",
				"err", resErr, "auth_request_id", authRequestID)
		} else {
			slog.InfoContext(ctx, "zitadellogin: handoff (uncollectible factor or unreadable policy/session)",
				"auth_request_id", authRequestID)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"handoff_url":     h.handoffURL(authRequestID),
			"auth_request_id": authRequestID,
		})
	}
}

// finishComplete resolves the subject of the now-sufficient session and runs
// the shared post-identity gauntlet via the injected CompleteFunc.
func (h *Handler) finishComplete(ctx context.Context, w http.ResponseWriter, res Result, sess Session, loginName, workspaceTenant string) {
	// Result carries no subject — re-read it. Zitadel's login_name is the
	// only identifier this package has for "email"; mark8ly logs in by
	// email, so the value the caller submitted to /zitadel/login is used
	// as-is rather than guessed at again here.
	factors, err := h.c.SessionFactors(ctx, sess.ID)
	if err != nil || factors.UserID == "" {
		slog.ErrorContext(ctx, "zitadellogin: could not resolve session subject after a sufficient decision", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}

	if h.complete == nil {
		slog.ErrorContext(ctx, "zitadellogin: no CompleteFunc configured")
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}
	if err := h.complete(ctx, w, factors.UserID, loginName, workspaceTenant); err != nil {
		slog.ErrorContext(ctx, "zitadellogin: post-identity gauntlet failed", "err", err, "user_id", factors.UserID)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"callback_url": res.CallbackURL})
}

// respondSessionCreateError maps CreatePasswordSession's errors.
//
// ErrBadCredentials and ErrUserNotFound MUST produce the identical response —
// collapsing them is the entire point of this function. Which one actually
// happened is logged for operators, never returned to the caller.
func (h *Handler) respondSessionCreateError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrBadCredentials):
		slog.WarnContext(ctx, "zitadellogin: login rejected: bad credentials")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_credentials"})
	case errors.Is(err, ErrUserNotFound):
		slog.WarnContext(ctx, "zitadellogin: login rejected: user not found")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_credentials"})
	case errors.Is(err, ErrUnavailable):
		slog.ErrorContext(ctx, "zitadellogin: zitadel unavailable creating session", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "zitadel_unavailable"})
	default:
		slog.ErrorContext(ctx, "zitadellogin: unexpected error creating session", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
	}
}

// respondTOTPVerifyError maps VerifyTOTP's errors. Unlike the login step,
// there is no enumeration concern here — the account is already established
// — but the wrong-code case still must never echo Zitadel's error body.
func (h *Handler) respondTOTPVerifyError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrBadCredentials):
		slog.WarnContext(ctx, "zitadellogin: totp rejected: bad code")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_totp"})
	case errors.Is(err, ErrUserNotFound):
		// The session itself vanished/expired between steps.
		slog.WarnContext(ctx, "zitadellogin: totp rejected: session not found")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_totp"})
	case errors.Is(err, ErrUnavailable):
		slog.ErrorContext(ctx, "zitadellogin: zitadel unavailable verifying totp", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "zitadel_unavailable"})
	default:
		slog.ErrorContext(ctx, "zitadellogin: unexpected error verifying totp", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal_error"})
	}
}

// handoffURL builds the Aurora-branded hosted login's continuation URL for an
// auth request this page decided it cannot (or should not) finish itself.
// Returns "" when no hosted login base URL was configured; the caller still
// gets auth_request_id back and is not stranded.
func (h *Handler) handoffURL(authRequestID string) string {
	if h.hostedLoginBaseURL == "" {
		return ""
	}
	return fmt.Sprintf("%s/ui/v2/login/login?authRequestID=%s", h.hostedLoginBaseURL, url.QueryEscape(authRequestID))
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, maxRequestBodyBytes)).Decode(v)
}

func writeJSON(w http.ResponseWriter, status int, v map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
