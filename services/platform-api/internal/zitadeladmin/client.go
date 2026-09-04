// Package zitadeladmin is the Zitadel v2-API implementation of the three
// account operations platform-api performs against an identity provider:
// send a password-reset code, redeem it, and delete an account. It exists
// to satisfy the interfaces defined in internal/auth (PasswordResetProvider)
// and internal/account (the unexported gipDeleter shape) — see D7 in
// docs/superpowers/specs/2026-09-03-zitadel-migration-design.md for the
// mapping this package implements.
//
// # Errors are gipadmin's sentinels, not new ones
//
// internal/auth/handler.go and internal/auth/service.go already branch with
// errors.Is against gipadmin.ErrUserNotFound, ErrInvalidOobCode,
// ErrWeakPassword, ErrUnauthenticated, ErrTooManyAttempts and ErrUnavailable.
// This package returns those SAME sentinel values (imported directly from
// gipadmin, not redeclared) rather than inventing a parallel set — the
// alternative would silently break every errors.Is check downstream the
// moment this provider is selected: a rate limit would read as a generic
// 500, a weak password would read as a server error, and so on, with no
// compiler or test signal pointing at why. Depending on gipadmin here for
// its sentinels only, not its behaviour, is deliberate: gipadmin itself is
// untouched by this package.
//
// # Request/response conventions
//
// Modelled directly on services/auth-bff/internal/zitadellogin/client.go:
// same requestOptions/do() shape, same withLogPath convention to keep
// caller-supplied ids out of error strings, same error-body extraction
// approach (readZitadelErrorID mirrors that package's readZitadelError).
//
// VERIFIED 2026-09-04 against the live TESSERIX Zitadel instance: a
// throwaway user was created, driven through the full password_reset ->
// password round trip, and deleted.
//
//   - Zitadel v2's protojson wire format flattens a oneof to its chosen
//     variant's field name directly, UN-NESTED — there is no wrapper key
//     named after the oneof itself. resolveUserIDByEmail's "emailQuery"
//     and the confirm call's "verificationCode" (both below) get this
//     right by construction. POST /v2/users/{id}/password_reset's "medium"
//     oneof is the same rule: the body is {"returnCode":{}} AT THE TOP
//     LEVEL, never {"medium":{"returnCode":{}}}. The wrapped form was
//     tried and VERIFIED WRONG: it returns 200 with NO verificationCode
//     (response keys == ["details"] only), and worse, Zitadel silently
//     falls back to sending ITS OWN unbranded reset email in that case —
//     the merchant gets an email nobody wrote AND this call still fails
//     with nothing to return. An earlier revision of this file sent the
//     wrapped form while this very doc comment already recorded the
//     correct one; the fixture in password_reset_test.go asserted the
//     wrapped shape too, so nothing caught it. If a future oneof field is
//     added to this package, send it un-nested and prove it with a test
//     that asserts the TOP-LEVEL key, not a wrapper.
//   - POST /v2/users/{id}/password_reset with {"returnCode":{}} returns 200
//     with {"details":…, "verificationCode":"<6 chars>"} — Zitadel hands
//     back the code and sends no notification of its own, exactly matching
//     D7's "return the code so we send the mail".
//   - POST /v2/users/{id}/password with
//     {"newPassword":{"password":…},"verificationCode":…} returns 200 on a
//     real, correct code.
//   - Error bodies carry a STABLE id at details[0].id, distinct from the
//     grpc code and from the message text, and NOT under one consistent
//     prefix — do not assume "COMMAND-" is the only shape:
//     user-not-found is COMMAND-SAF4f (404, code 5); a wrong/expired
//     verification code is COMMAND-2M9fs (400, code 9, message "Code not
//     found"); a request missing the password field is COMMAND-G8dh3
//     (400, code 9, message "Password not found") — these two share both
//     HTTP status and grpc code and differ in message by one word; a
//     too-short password is DOMAIN-HuJf6 (400, code 3, message "Password
//     is too short") — a message that hits none of classifyError's
//     substring fallbacks, so relying on message text alone for this case
//     would have surfaced as ErrUnavailable ("identity service
//     unreachable") instead of ErrWeakPassword. classifyError keys off
//     details[0].id first for exactly these reasons; see its doc.
//
// DELETE /v2/users/{id} was separately marked VERIFIED in the D7 spec
// table, and re-confirmed in the same round trip.
//
// The /v2/users search shape was also probed live on 2026-09-04: POST
// /v2/users with {"queries":[{"emailQuery":{"emailAddress":...,
// "method":"TEXT_QUERY_METHOD_EQUALS_IGNORE_CASE"}}]} returns matches under
// "result". A ZERO-match search omits "result" entirely rather than
// returning an empty array — protojson elides empty values — which decodes
// to a nil slice here, so the len()==0 path below is the correct one to
// rely on. Do not rewrite it to test for the key's presence.
//
// What remains best-effort: any error id not in the table above.
package zitadeladmin

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mark8ly/platform-api/internal/gipadmin"
)

const (
	defaultTimeout      = 15 * time.Second
	maxSuccessBodyBytes = 64 * 1024
	maxErrorBodyBytes   = 4096
)

// ErrAmbiguousEmail is returned when an email search matches more than one
// Zitadel user. It is NOT one of the sentinels internal/auth/handler.go
// branches on, and that is deliberate: an ambiguous match is a data
// integrity problem, not a missing user, a bad code, a weak password, or an
// unreachable upstream. Falling through handler.go's switch to its default
// case (500, logged) is the honest outcome — telling the caller "not found"
// would be false, and "unavailable" would misdirect anyone investigating
// the log line toward a network problem that doesn't exist. See
// resolveUserIDByEmail's doc for why this refuses rather than picking one.
var ErrAmbiguousEmail = errors.New("zitadeladmin: more than one user matched the email")

// Config holds the values needed to construct a Client.
type Config struct {
	// BaseURL is the Zitadel instance origin, e.g. "https://auth.tesserix.app".
	BaseURL string
	// Token is the service-user/login-client personal access token used to
	// authenticate every call in this package. It is instance-level and
	// must never be logged.
	Token string
	// OrgID scopes the email->user-id search (see resolveUserIDByEmail).
	// D1 in the migration spec fixes exactly one org, TESSERIX, for this
	// whole instance, but the instance is shared with other products (the
	// package doc on services/auth-bff/internal/zitadellogin/client.go
	// notes the same PAT can mint sessions for "any user of any product on
	// the shared instance") — so an unscoped search could still match a
	// same-address user belonging to a different product's org. Required;
	// New refuses an empty value rather than searching instance-wide, the
	// same refusal zitadellogin.FindUserByVerifiedEmail makes.
	OrgID string
}

// Client is a thin Zitadel v2 API client for the three account operations
// platform-api performs against an identity provider.
type Client struct {
	baseURL string
	token   string
	orgID   string
	hc      *http.Client
}

// New constructs a Client. hc may be nil, in which case a client with
// defaultTimeout is used.
func New(cfg Config, hc *http.Client) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("zitadeladmin: BaseURL is required")
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("zitadeladmin: Token is required")
	}
	if cfg.OrgID == "" {
		return nil, fmt.Errorf("zitadeladmin: OrgID is required")
	}
	if hc == nil {
		hc = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{
		baseURL: strings.TrimSuffix(cfg.BaseURL, "/"),
		token:   cfg.Token,
		orgID:   cfg.OrgID,
		hc:      hc,
	}, nil
}

// requestOptions accumulates per-request settings, mirroring
// zitadellogin's shape and its reasoning: an option type over *http.Request
// itself could clobber the Authorization header set from the token.
type requestOptions struct {
	// logPath replaces path in every error/log string do() builds, WITHOUT
	// affecting the actual HTTP request. Defaults to path. Used to keep a
	// caller-supplied Zitadel user id out of logs — see withLogPath.
	logPath string
}

type requestOption func(*requestOptions)

func withLogPath(label string) requestOption {
	return func(ro *requestOptions) { ro.logPath = label }
}

// do performs one HTTP round trip and maps a non-2xx response to one of
// gipadmin's sentinel errors via classifyError. out may be nil when the
// caller doesn't need the response body decoded.
func (c *Client) do(ctx context.Context, method, path string, body, out any, scopeToOrg bool, opts ...requestOption) error {
	ro := requestOptions{logPath: path}
	for _, opt := range opts {
		opt(&ro)
	}
	logPath := ro.logPath

	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("zitadeladmin: marshal %s %s: %w", method, logPath, err)
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return fmt.Errorf("zitadeladmin: build %s %s: %w", method, logPath, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	if scopeToOrg {
		req.Header.Set("x-zitadel-orgid", c.orgID)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("zitadeladmin: %s %s: %v: %w", method, logPath, err, gipadmin.ErrUnavailable)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		id := readZitadelErrorID(respBody)
		var parsed zitadelErrorBody
		_ = json.Unmarshal(respBody, &parsed) // best-effort; a malformed body just leaves the zero value
		return &apiError{
			method:   method,
			logPath:  logPath,
			status:   resp.StatusCode,
			id:       id,
			grpcCode: parsed.Code,
			message:  parsed.Message,
			sentinel: classifyError(resp.StatusCode, respBody, id),
		}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxSuccessBodyBytes)).Decode(out); err != nil {
		return fmt.Errorf("zitadeladmin: decode %s %s: %v: %w", method, logPath, err, gipadmin.ErrUnavailable)
	}
	return nil
}

// apiError is the error do() returns for every non-2xx Zitadel response.
//
// It exists so a caller can branch on Zitadel's STABLE details[0].id
// (see readZitadelErrorID and zitadelErrorIDSentinels) without parsing
// an error string. EnsureHumanUser needs exactly that: "this email is
// already a Zitadel user" (id V3-DKcYh) is a resolvable condition, not a
// failure, and it arrives as a 409 that classifyError deliberately maps
// to the coarse ErrUnavailable — indistinguishable from a real outage if
// all you have is the sentinel.
//
// Error() reproduces byte-for-byte the string do() used to build with
// fmt.Errorf, and Unwrap returns the same gipadmin sentinel, so every
// existing errors.Is check downstream (internal/auth/handler.go and
// service.go branch on six of them) keeps behaving identically.
type apiError struct {
	method   string
	logPath  string
	status   int
	id       string
	grpcCode int
	message  string
	sentinel error
}

func (e *apiError) Error() string {
	return fmt.Sprintf("zitadeladmin: %s %s: status %d: %s: %v", e.method, e.logPath, e.status, e.id, e.sentinel)
}

func (e *apiError) Unwrap() error { return e.sentinel }

// errorID returns Zitadel's stable details[0].id for err, or "" when err
// did not come from a Zitadel response (or carried no id). "" must never
// be compared against a real id — treat it only as "no id available".
func errorID(err error) string {
	var ae *apiError
	if errors.As(err, &ae) {
		return ae.id
	}
	return ""
}

// zitadelErrorBody is the Zitadel v2 grpc-gateway error envelope:
// {"code": <grpc status code>, "message": "...", "details": [{"id": "..."}]}.
type zitadelErrorBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Details []struct {
		ID string `json:"id"`
	} `json:"details"`
}

// readZitadelErrorID extracts ONLY details[0].id from a Zitadel error
// body, mirroring services/auth-bff/internal/zitadellogin/client.go's
// readZitadelError (that function also returns the grpc code; this
// package doesn't currently need it separately, so classifyError re-parses
// the body itself for code+message). Returns "" when absent or
// undecodable — the caller must never treat "" as if it were a real,
// unrecognized id, only as "no id available, fall back to the coarser
// mapping".
func readZitadelErrorID(body []byte) string {
	var e zitadelErrorBody
	if err := json.Unmarshal(body, &e); err != nil {
		return ""
	}
	if len(e.Details) > 0 {
		return e.Details[0].ID
	}
	return ""
}

// zitadelErrorIDSentinels maps a Zitadel error's stable details[0].id to
// the gipadmin sentinel it means, for ids VERIFIED live on 2026-09-04
// against the TESSERIX instance (see this file's package doc). This is
// the PRIMARY mapping — classifyError only falls back to HTTP status +
// message-substring matching when the id is absent or not in this table.
//
// Ids do NOT share a single prefix ("COMMAND-" and "DOMAIN-" both occur
// live) — never assume one when adding a new entry.
//
// COMMAND-2M9fs and COMMAND-G8dh3 are both 400s with grpc code 9 and
// messages that differ by a single word ("Code not found" vs "Password
// not found") — text matching alone cannot reliably tell a wrong/expired
// verification code apart from a malformed request missing the password
// field, and getting that distinction wrong is exactly the silent
// degradation this package exists to avoid: a malformed request must never
// read as "your reset code is invalid" to the merchant. COMMAND-G8dh3
// therefore maps to ErrUnavailable (a server-side request-shape bug on our
// end, not a bad code), never to ErrInvalidOobCode.
var zitadelErrorIDSentinels = map[string]error{
	"COMMAND-SAF4f": gipadmin.ErrUserNotFound,   // "User could not be found" (404, code 5)
	"COMMAND-2M9fs": gipadmin.ErrInvalidOobCode, // "Code not found" (400, code 9)
	"COMMAND-G8dh3": gipadmin.ErrUnavailable,    // "Password not found" (400, code 9) -- malformed request, NOT an invalid code
	"DOMAIN-HuJf6":  gipadmin.ErrWeakPassword,   // "Password is too short" (400, code 3)
}

// classifyError maps an HTTP status + Zitadel error body to one of
// gipadmin's sentinels. id (from readZitadelErrorID) is checked first
// against zitadelErrorIDSentinels; it is the reliable signal because it is
// stable and distinguishes cases the message text alone cannot (see that
// map's doc). When id is absent or not one seen live, this falls back to
// the coarser HTTP-status mapping (404 -> not found, 401/403 ->
// unauthenticated, 429 -> too many attempts) and, within an unrecognized
// 400, a message-substring safety net. A 400 that matches neither the id
// table nor a known substring maps to ErrUnavailable rather than guessing:
// a wrong guess here reads as "wrong code" or "weak password" to the
// merchant when it is neither.
func classifyError(status int, body []byte, id string) error {
	if id != "" {
		if sentinel, ok := zitadelErrorIDSentinels[id]; ok {
			return sentinel
		}
	}

	var e zitadelErrorBody
	_ = json.Unmarshal(body, &e) // best-effort; a malformed body just yields Message == ""
	msg := strings.ToLower(e.Message)

	switch status {
	case http.StatusNotFound:
		return gipadmin.ErrUserNotFound
	case http.StatusUnauthorized, http.StatusForbidden:
		return gipadmin.ErrUnauthenticated
	case http.StatusTooManyRequests:
		return gipadmin.ErrTooManyAttempts
	case http.StatusBadRequest:
		switch {
		case strings.Contains(msg, "password") &&
			(strings.Contains(msg, "weak") || strings.Contains(msg, "polic") || strings.Contains(msg, "complexity")):
			return gipadmin.ErrWeakPassword
		case strings.Contains(msg, "code") && !strings.Contains(msg, "password") &&
			(strings.Contains(msg, "invalid") || strings.Contains(msg, "expired") || strings.Contains(msg, "not found")):
			// "not found" is included for the real observed message ("Code
			// not found"), but only when "password" is absent — the
			// COMMAND-G8dh3 case ("Password not found") must never match
			// here. This is now a SAFETY NET behind the id table above,
			// not the primary signal.
			return gipadmin.ErrInvalidOobCode
		default:
			return gipadmin.ErrUnavailable
		}
	default:
		return gipadmin.ErrUnavailable
	}
}

// compositeCode packs a Zitadel user id together with the verification
// code Zitadel hands back from POST /v2/users/{id}/password_reset.
//
// Why this exists: Zitadel's confirm call, POST /v2/users/{id}/password,
// needs BOTH the user id and the verification code. GIP's oobCode alone is
// proof of possession and needs no accompanying user id. But
// auth.PasswordResetProvider's interface — frozen by task 1 — is a plain
// string out of SendPasswordResetOobCode and a plain string in on
// ResetPassword's oobCode parameter, with no room for a second field. That
// string is also NOT free-form: auth.Service.buildResetURL appends it,
// unescaped, straight into a URL query value
// ("...?oobCode=" + oobCode). So whatever this package returns from
// SendPasswordResetOobCode must itself already be a single, URL-safe
// string carrying both pieces.
//
// Base64 (URL alphabet, unpadded) over a small JSON envelope is used
// rather than a delimited string like "userID:code": RawURLEncoding's
// alphabet (letters, digits, '-', '_') is a strict subset of characters
// valid in an unescaped URL query value, whereas neither a Zitadel user id
// nor a verification code is documented to exclude ':' — a delimited join
// could be ambiguous to split back apart.
type compositeCode struct {
	UserID string `json:"u"`
	Code   string `json:"c"`
}

func encodeCompositeCode(userID, code string) string {
	// compositeCode's fields are plain strings; json.Marshal cannot fail
	// on this input.
	raw, _ := json.Marshal(compositeCode{UserID: userID, Code: code})
	return base64.RawURLEncoding.EncodeToString(raw)
}

// decodeCompositeCode reverses encodeCompositeCode. Any failure — bad
// base64, bad JSON, or a missing field — maps to gipadmin.ErrInvalidOobCode:
// from the caller's perspective a malformed reset code and an
// expired/redeemed one are the same outcome, "this link doesn't work,
// request a new one," and handler.go already renders that as 410 Gone.
func decodeCompositeCode(s string) (userID, code string, err error) {
	raw, decErr := base64.RawURLEncoding.DecodeString(s)
	if decErr != nil {
		return "", "", fmt.Errorf("zitadeladmin: decode reset code: %w", gipadmin.ErrInvalidOobCode)
	}
	var cc compositeCode
	if jsonErr := json.Unmarshal(raw, &cc); jsonErr != nil || cc.UserID == "" || cc.Code == "" {
		return "", "", fmt.Errorf("zitadeladmin: malformed reset code: %w", gipadmin.ErrInvalidOobCode)
	}
	return cc.UserID, cc.Code, nil
}

// resolveUserIDByEmail searches for a Zitadel user by email, scoped to
// Config.OrgID, and returns exactly the sole matching user id.
//
// Zero matches maps to gipadmin.ErrUserNotFound: RequestPasswordReset in
// internal/auth/service.go already does
// `errors.Is(err, gipadmin.ErrUserNotFound)` to suppress account
// enumeration, and this keeps that behaviour working unchanged with this
// provider swapped in.
//
// More than one match is a REFUSAL, not a "pick the first one": which user
// a search returns first for an ambiguous query is unspecified ordering,
// and silently acting on one of several matches for a destructive,
// user-visible operation like password reset is exactly the kind of
// decision this function must not make on a caller's behalf — mirrors
// zitadellogin.FindUserByVerifiedEmail's ErrAmbiguousEmailMatch for the
// identical reason. See ErrAmbiguousEmail's doc for why it is not one of
// gipadmin's sentinels.
//
// Matching requires BOTH a case-insensitive email match AND
// human.email.isVerified — the same pair zitadellogin.FindUserByVerifiedEmail
// uses. D7's own note that "email.isVerified must be set deliberately" for
// every user this system creates (signup and admin-invite alike) means
// every legitimate account already satisfies this; requiring it defends
// against redeeming a password reset against an email Zitadel itself has
// not attested to.
func (c *Client) resolveUserIDByEmail(ctx context.Context, email string) (string, error) {
	body := map[string]any{
		"queries": []any{
			map[string]any{
				"emailQuery": map[string]any{
					"emailAddress": email,
					"method":       "TEXT_QUERY_METHOD_EQUALS_IGNORE_CASE",
				},
			},
		},
	}
	var wire struct {
		Result []struct {
			UserID string `json:"userId"`
			Human  *struct {
				Email struct {
					Email      string `json:"email"`
					IsVerified bool   `json:"isVerified"`
				} `json:"email"`
			} `json:"human"`
		} `json:"result"`
	}
	if err := c.do(ctx, http.MethodPost, "/v2/users", body, &wire, true, withLogPath("/v2/users (search)")); err != nil {
		return "", err
	}

	var matches []string
	for _, u := range wire.Result {
		if u.Human != nil && strings.EqualFold(u.Human.Email.Email, email) && u.Human.Email.IsVerified {
			matches = append(matches, u.UserID)
		}
	}
	switch len(matches) {
	case 0:
		return "", gipadmin.ErrUserNotFound
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("zitadeladmin: %d users matched the email: %w", len(matches), ErrAmbiguousEmail)
	}
}

// SendPasswordResetOobCode resolves email to a Zitadel user id, then asks
// Zitadel to mint a password-reset verification code WITHOUT sending its
// own notification (the "returnCode" medium — see D7: this preserves
// today's GIP returnOobLink=true behaviour and keeps branding/delivery on
// platform-api's own notify path). The returned string packs the user id
// and code together; see compositeCode's doc. It is opaque to every other
// caller and must be passed back to ResetPassword unmodified.
//
// The request body's "returnCode" key is sent at the TOP LEVEL, not
// wrapped under a "medium" key. medium is a protojson oneof, and Zitadel
// v2's protojson wire format flattens oneofs to their chosen variant's
// field name directly — resolveUserIDByEmail's un-nested "emailQuery" and
// this function's own un-nested "verificationCode" on the confirm call
// already get this right. A body wrapping it in "medium" was tried and
// VERIFIED WRONG live: it returns 200 with no verificationCode (body keys
// == ["details"] only) — worse than an error, because Zitadel silently
// falls back to sending ITS OWN unbranded reset notification and hands
// this package nothing, so the merchant gets an email nobody wrote AND
// this call then fails upstream with no code to return.
func (c *Client) SendPasswordResetOobCode(ctx context.Context, email string) (string, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return "", fmt.Errorf("zitadeladmin: email is required")
	}

	userID, err := c.resolveUserIDByEmail(ctx, email)
	if err != nil {
		return "", err
	}

	var wire struct {
		VerificationCode string `json:"verificationCode"`
	}
	path := fmt.Sprintf("/v2/users/%s/password_reset", url.PathEscape(userID))
	body := map[string]any{
		"returnCode": map[string]any{},
	}
	if err := c.do(ctx, http.MethodPost, path, body, &wire, false, withLogPath("/v2/users/{id}/password_reset")); err != nil {
		return "", err
	}
	if wire.VerificationCode == "" {
		return "", fmt.Errorf("zitadeladmin: password_reset 2xx without a verificationCode: %w", gipadmin.ErrUnavailable)
	}
	return encodeCompositeCode(userID, wire.VerificationCode), nil
}

// ResetPassword decodes oobCode back into the user id + verification code
// SendPasswordResetOobCode packed together, then submits the new password
// to Zitadel. A malformed oobCode never reaches the network — it maps
// straight to gipadmin.ErrInvalidOobCode, same as a code Zitadel itself
// rejects as invalid or expired.
func (c *Client) ResetPassword(ctx context.Context, oobCode, newPassword string) error {
	userID, code, err := decodeCompositeCode(strings.TrimSpace(oobCode))
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/v2/users/%s/password", url.PathEscape(userID))
	body := map[string]any{
		"newPassword":      map[string]any{"password": newPassword},
		"verificationCode": code,
	}
	return c.do(ctx, http.MethodPost, path, body, nil, false, withLogPath("/v2/users/{id}/password"))
}

// DeleteAccount removes the Zitadel user identified by uid. Idempotent:
// gipadmin.ErrUserNotFound (a 404) is treated as success, mirroring
// gipadmin.AdminClient.DeleteAccount's own doc — account deletion is
// retried and the user may already be gone.
func (c *Client) DeleteAccount(ctx context.Context, uid string) error {
	if uid == "" {
		return fmt.Errorf("zitadeladmin: uid is required")
	}
	path := fmt.Sprintf("/v2/users/%s", url.PathEscape(uid))
	err := c.do(ctx, http.MethodDelete, path, nil, nil, false, withLogPath("/v2/users/{id}"))
	if errors.Is(err, gipadmin.ErrUserNotFound) {
		return nil
	}
	return err
}
