// Package zitadellogin speaks Zitadel's v2 login-client HTTP API so auth-bff
// can host its own login page instead of redirecting to Zitadel's.
//
// Every JSON shape and error mapping here is pinned to what was OBSERVED
// against a live Zitadel v4.15.3 instance, not to what the documentation says
// the API should return. Three separate shapes in this API answer 200 or 201
// while doing something other than what the caller assumes.
//
// This package makes NO decision about whether a session is adequate to
// finalize a login — that is sufficiency.go's job, and Zitadel will happily
// issue an authorization code for a password-only session even under a
// forceMfa policy.
package zitadellogin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultTimeout      = 10 * time.Second
	maxSuccessBodyBytes = 64 * 1024
	maxErrorBodyBytes   = 4096
)

var (
	ErrBadCredentials     = errors.New("zitadellogin: bad credentials")
	ErrUserNotFound       = errors.New("zitadellogin: user not found")
	ErrAuthRequestInvalid = errors.New("zitadellogin: auth request invalid")
	ErrUnavailable        = errors.New("zitadellogin: zitadel unavailable")

	// ErrAmbiguousEmailMatch is returned by FindUserByVerifiedEmail when
	// more than one user in the org matches the searched email. This is a
	// refusal, not a fallback to "pick the first one": which user Zitadel
	// returns first for an ambiguous query is unspecified ordering, and a
	// caller who trusted it would let whichever account ties or races into
	// existence first win control of a genuinely different person's
	// federated sign-in.
	ErrAmbiguousEmailMatch = errors.New("zitadellogin: more than one user matched the verified email")

	// ErrEmailAlreadyExists is CreateHumanUserWithIDPLink's distinguished
	// mapping for Zitadel's duplicate-email rejection (a 400 from
	// AddHumanUser when the email is already taken by another user, most
	// often the same email in a different case than a case-insensitive
	// FindUserByVerifiedEmail search already ruled out — but a caller that
	// skips that search, or hits a race, still needs a caller-actionable
	// outcome instead of a generic ErrBadCredentials/ErrUnavailable that
	// reads as "wrong password" or "try again later" for what is neither.
	ErrEmailAlreadyExists = errors.New("zitadellogin: email already exists")

	// ErrWeakPassword is CreateHumanUserWithPassword's distinguished mapping
	// for Zitadel's password-complexity rejection: details[0].id ==
	// "DOMAIN-HuJf6" ("Password is too short"), verified live against the
	// TESSERIX instance in phase 5 (see
	// services/platform-api/internal/zitadeladmin/client.go's package doc).
	// Kept separate from ErrEmailAlreadyExists because AddHumanUser can
	// answer 400 for either reason on the SAME call, and a weak password
	// must never read to a caller as "that email is taken" or as a generic
	// failure — both are false, and false-negative password copy is
	// exactly the defect this sentinel exists to prevent.
	ErrWeakPassword = errors.New("zitadellogin: password does not meet policy")
)

// weakPasswordErrorID is the stable details[0].id Zitadel returns for a
// too-short/too-weak password on AddHumanUser. See ErrWeakPassword's doc:
// this id does NOT share a prefix with the "DOMAIN-"/"COMMAND-" ids seen
// elsewhere in this package, so it must be matched exactly, never by
// prefix.
const weakPasswordErrorID = "DOMAIN-HuJf6"

// duplicateEmailErrorID is the stable details[0].id observed for
// AddHumanUser's duplicate-email (ALREADY_EXISTS) rejection — the same id
// CreateHumanUserWithIDPLink's tests pin. CreateHumanUserWithPassword keys
// off this id FIRST and grpc code 6 only as a fallback (see its classify
// closure) so an unrecognized future ALREADY_EXISTS id still maps
// correctly, and so this narrowing cannot accidentally also catch the
// weak-password case, which also uses a low grpc code but a completely
// different id.
const duplicateEmailErrorID = "COMMAND-oR9nS"

type Client struct {
	baseURL string
	token   string
	hc      *http.Client
}

// New builds a client. token is the login-client PAT: it authenticates every
// call in this package, never the end-user session being established. It is
// instance-level and can mint a session for any user of any product on the
// shared instance — treat it as the most powerful credential this service
// holds and never log it.
func New(baseURL, token string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{baseURL: strings.TrimSuffix(baseURL, "/"), token: token, hc: hc}
}

// Session is a Zitadel session. Token is a bearer-equivalent secret for this
// one session and must never be logged.
type Session struct {
	ID    string
	Token string
}

// LoginPolicy carries the two MFA fields as SEPARATE values. They are not
// folded together: forceMfaLocalOnly means "require MFA for local/password
// users, not federated ones", and mark8ly has federated Google and Apple
// users for whom that distinction is load-bearing.
type LoginPolicy struct {
	ForceMFA          bool
	ForceMFALocalOnly bool
}

type Factors struct {
	Password bool
	TOTP     bool
	UserID   string
	OrgID    string
}

type AuthRequest struct {
	ID string
}

// requestOptions accumulates per-request settings. The option type is
// func(*requestOptions), NOT func(*http.Request), deliberately: an option that
// could reach the raw request could set or overwrite any header — including
// the Authorization header do sets from the PAT. This shape makes that class
// of bug unrepresentable rather than something a reviewer must keep checking.
type requestOptions struct {
	orgID string
	// badRequestErr overrides do()'s default ErrBadCredentials mapping for
	// an HTTP 400. Most callers want the default (a 400 from a
	// credential-shaped call really does mean "bad credentials").
	badRequestErr error
	// badRequestCode, when non-zero, narrows badRequestErr to apply ONLY
	// when the response body's top-level grpc-style `code` field equals
	// it; any other 400 falls back to ErrUnavailable instead. Without this,
	// withBadRequestError(ErrEmailAlreadyExists) on AddHumanUser turned
	// EVERY 400 from that call — a malformed profile field, a future
	// Zitadel validation change, any policy rejection — into
	// "email already exists", which is both wrong and, worse, reads as a
	// transient race a caller could retry past when it is neither. Set via
	// withBadRequestErrorForCode; zero (the default) means "no narrowing",
	// preserving the unconditional behaviour every other caller of
	// withBadRequestError still wants.
	badRequestCode int
	// badRequestClassifier, when set, TAKES OVER the entire 400 mapping
	// from badRequestErr/badRequestCode: it is called with the response
	// body's grpc-style code and details[0].id and must return the error
	// to use, defaulting unmatched cases to ErrUnavailable itself. This
	// exists because a single call can need to distinguish more than one
	// specific 400 case that do NOT share a grpc code — e.g.
	// CreateHumanUserWithPassword's AddHumanUser 400s on both a
	// too-short password (code 3, id DOMAIN-HuJf6) and a duplicate email
	// (code 6, id COMMAND-oR9nS) — which withBadRequestErrorForCode's
	// single (code, err) pair cannot express without conflating a weak
	// password with any other unrelated code-3 rejection.
	badRequestClassifier func(code int, id string) error
	// logPath replaces path in every error string do() builds, WITHOUT
	// affecting the actual HTTP request (which always uses the real path).
	// Defaults to path itself. Use withLogPath when path carries a
	// caller-supplied id that should not ride along into logs — e.g. an
	// idp intent id, which is not a secret the way its token is, but is
	// still request-scoped input this package otherwise takes care never
	// to echo unnecessarily.
	logPath string
}

type requestOption func(*requestOptions)

func withOrgID(orgID string) requestOption {
	return func(ro *requestOptions) { ro.orgID = orgID }
}

func withBadRequestError(err error) requestOption {
	return func(ro *requestOptions) { ro.badRequestErr = err }
}

// withBadRequestErrorForCode is withBadRequestError narrowed to one
// grpc-style error code: a 400 whose body's `code` field matches maps to
// err; any other 400 maps to ErrUnavailable instead (see badRequestCode's
// doc for why this narrowing exists). code 6 is grpc's ALREADY_EXISTS —
// observed on Zitadel's AddHumanUser duplicate-email rejection (see
// client_test.go).
func withBadRequestErrorForCode(code int, err error) requestOption {
	return func(ro *requestOptions) {
		ro.badRequestErr = err
		ro.badRequestCode = code
	}
}

func withLogPath(label string) requestOption {
	return func(ro *requestOptions) { ro.logPath = label }
}

// withBadRequestClassifier installs a full (code, id) -> error mapping for a
// 400 response, overriding withBadRequestError/withBadRequestErrorForCode
// for this call. See badRequestClassifier's field doc for why this exists.
func withBadRequestClassifier(classify func(code int, id string) error) requestOption {
	return func(ro *requestOptions) { ro.badRequestClassifier = classify }
}

func (c *Client) do(ctx context.Context, method, path string, body, out any, notFound error, opts ...requestOption) error {
	ro := requestOptions{badRequestErr: ErrBadCredentials, logPath: path}
	for _, opt := range opts {
		opt(&ro)
	}
	logPath := ro.logPath

	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("zitadellogin: marshal %s %s: %w", method, logPath, err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return fmt.Errorf("zitadellogin: build %s %s: %w", method, logPath, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	if ro.orgID != "" {
		req.Header.Set("x-zitadel-orgid", ro.orgID)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("zitadellogin: %s %s: %v: %w", method, logPath, err, ErrUnavailable)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		code, id := readZitadelError(resp.Body)
		switch {
		case resp.StatusCode == http.StatusBadRequest:
			badReqErr := ro.badRequestErr
			switch {
			case ro.badRequestClassifier != nil:
				badReqErr = ro.badRequestClassifier(code, id)
			case ro.badRequestCode != 0 && code != ro.badRequestCode:
				// Narrowed mapping configured (withBadRequestErrorForCode)
				// but this 400 is not the specific case it targets — refuse
				// to guess, fall back to a generic failure rather than
				// mislabel it.
				badReqErr = ErrUnavailable
			}
			return fmt.Errorf("zitadellogin: %s %s: %s: %w", method, logPath, id, badReqErr)
		case resp.StatusCode == http.StatusNotFound:
			return fmt.Errorf("zitadellogin: %s %s: %s: %w", method, logPath, id, notFound)
		default:
			return fmt.Errorf("zitadellogin: %s %s: status %d: %s: %w", method, logPath, resp.StatusCode, id, ErrUnavailable)
		}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxSuccessBodyBytes)).Decode(out); err != nil {
		return fmt.Errorf("zitadellogin: decode %s %s: %v: %w", method, logPath, err, ErrUnavailable)
	}
	return nil
}

// readZitadelError extracts ONLY the top-level grpc-style `code` and
// details[0].id (e.g. "COMMAND-3M0fs") from a Zitadel error body. The raw
// error body is never surfaced, because that is exactly where Zitadel puts
// failedAttempts — a counter that must not reach a caller or a log line.
// code is 0 when absent or undecodable — the same "unknown" convention id
// already uses, so a caller narrowing on a specific code (see
// withBadRequestErrorForCode) never confuses "no code in the body" with a
// genuine code 0 (which grpc does not assign to any status).
func readZitadelError(r io.Reader) (code int, id string) {
	var wire struct {
		Code    int `json:"code"`
		Details []struct {
			ID string `json:"id"`
		} `json:"details"`
	}
	if err := json.NewDecoder(io.LimitReader(r, maxErrorBodyBytes)).Decode(&wire); err != nil {
		return 0, "unknown"
	}
	id = "unknown"
	if len(wire.Details) > 0 && wire.Details[0].ID != "" {
		id = wire.Details[0].ID
	}
	return wire.Code, id
}

func (c *Client) AuthRequest(ctx context.Context, id string) (AuthRequest, error) {
	var wire struct {
		AuthRequest struct {
			ID string `json:"id"`
		} `json:"authRequest"`
	}
	err := c.do(ctx, http.MethodGet, "/v2/oidc/auth_requests/"+url.PathEscape(id), nil, &wire, ErrAuthRequestInvalid)
	if err != nil {
		return AuthRequest{}, err
	}
	return AuthRequest{ID: wire.AuthRequest.ID}, nil
}

// CreatePasswordSession checks the login name and password in ONE call, so a
// wrong username and a wrong password take the same code path and the same
// time. ErrUserNotFound and ErrBadCredentials stay distinct here for logging;
// collapsing them into one user-facing answer is the handler's job.
func (c *Client) CreatePasswordSession(ctx context.Context, loginName, password string) (Session, error) {
	body := map[string]any{
		"checks": map[string]any{
			"user":     map[string]any{"loginName": loginName},
			"password": map[string]any{"password": password},
		},
	}
	var wire struct {
		SessionID    string `json:"sessionId"`
		SessionToken string `json:"sessionToken"`
	}
	if err := c.do(ctx, http.MethodPost, "/v2/sessions", body, &wire, ErrUserNotFound); err != nil {
		return Session{}, err
	}
	return Session{ID: wire.SessionID, Token: wire.SessionToken}, nil
}

// CreateIDPIntentSession creates a Zitadel session from a resolved IDP
// intent (see RetrieveIDPIntent), mirroring CreatePasswordSession's shape but
// with checks.idpIntent instead of checks.user + checks.password.
//
// Zitadel resolves the session's subject itself from the intent — this call
// carries no user id, so a caller who only holds a genuine intent id/token
// pair (never a claimed user id) is the only way to reach a session here.
// The intent must already be linked to an existing Zitadel user (see
// IDPIdentity.ZitadelUserID's doc); callers are expected to have checked
// that via RetrieveIDPIntent before calling this.
func (c *Client) CreateIDPIntentSession(ctx context.Context, intentID, intentToken string) (Session, error) {
	body := map[string]any{
		"checks": map[string]any{
			"idpIntent": map[string]any{
				"idpIntentId":    intentID,
				"idpIntentToken": intentToken,
			},
		},
	}
	var wire struct {
		SessionID    string `json:"sessionId"`
		SessionToken string `json:"sessionToken"`
	}
	if err := c.do(ctx, http.MethodPost, "/v2/sessions", body, &wire, ErrUserNotFound); err != nil {
		return Session{}, err
	}
	return Session{ID: wire.SessionID, Token: wire.SessionToken}, nil
}

// FindUserByVerifiedEmail searches for an EXISTING Zitadel user, WITHIN
// orgID, whose email matches case-insensitively and whose email Zitadel
// itself has separately marked verified. Returns "" with a nil error when
// no such user exists — an ordinary, expected outcome for a first-time
// federated identity, not a failure.
//
// Two things this function refuses to do, both load-bearing:
//
//   - Search outside orgID. The login-client PAT behind this call is
//     instance-level and Zitadel's email uniqueness is enforced per-org, so
//     two different orgs can each hold a verified "victim@x.com" — an
//     unscoped search would let an account in a completely unrelated org
//     (one this merchant has no relationship to) win an ambiguous match.
//     orgID is required; an empty one refuses rather than searching
//     instance-wide.
//   - Pick a match when more than one exists. Which user Zitadel would
//     return first for an ambiguous query is unspecified ordering, not a
//     decision this code is willing to make on a caller's behalf — see
//     ErrAmbiguousEmailMatch.
//
// Case-insensitive matching (TEXT_QUERY_METHOD_EQUALS_IGNORE_CASE + Go-side
// strings.EqualFold) is deliberate, not a nicety: Zitadel's own account
// uniqueness is case-insensitive, so a case-SENSITIVE search here would
// simply never find "Person@x.com" for Google's "person@x.com" — reading as
// "no match" when the account is right there under a different case.
//
// Verified 2026-09-04 against the live TESSERIX Zitadel instance (see
// README) for the un-scoped, case-sensitive shape; the org-scoping header
// and the IGNORE_CASE method follow the same documented v2
// UserService.ListUsers request shape and were not independently
// re-verified live.
func (c *Client) FindUserByVerifiedEmail(ctx context.Context, orgID, email string) (string, error) {
	if orgID == "" {
		return "", fmt.Errorf("zitadellogin: FindUserByVerifiedEmail with an empty org id, refusing rather than searching instance-wide: %w", ErrUnavailable)
	}
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
	if err := c.do(ctx, http.MethodPost, "/v2/users", body, &wire, ErrUnavailable, withOrgID(orgID)); err != nil {
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
		return "", nil
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("zitadellogin: %d users matched the verified email: %w", len(matches), ErrAmbiguousEmailMatch)
	}
}

// CreateHumanUserWithIDPLink registers a brand-new Zitadel human user
// pre-linked to the given federated identity, so the VERY NEXT retrieve of
// the same provider identity resolves IDPIdentity.ZitadelUserID immediately
// — no second registration, no duplicate account.
//
// This is a genuine account-creation primitive and is guarded accordingly:
// it refuses outright unless identity carries a non-empty, verified email.
// Callers must ALSO have already checked FindUserByVerifiedEmail before
// calling this — creating here without that check is how one person ends
// up as two disconnected Zitadel accounts.
//
// Modelled on Zitadel's documented v2 UserService.AddHumanUser
// ("POST /v2/users/human") request shape: profile + email + idpLinks. NOT
// directly observed against a live instance at the time this was written —
// see this package's README for the "observed vs documented" convention.
// Deliberately does NOT use the deprecated addHumanUser field on the
// idp-intent retrieve/session-create calls elsewhere in this package —
// Zitadel's own API docs mark that field deprecated in favour of this
// explicit, separate creation call.
func (c *Client) CreateHumanUserWithIDPLink(ctx context.Context, identity IDPIdentity) (string, error) {
	if identity.Email == "" || !identity.EmailVerified {
		return "", fmt.Errorf("zitadellogin: refusing to create a user from an empty or unverified email: %w", ErrUnavailable)
	}

	// Zitadel requires non-empty given/family names on a human profile.
	// Prefer Google's own given_name/family_name claims (identity.GivenName/
	// FamilyName — read best-effort from rawInformation, see readRawName)
	// when the provider sent them; a shopper's real name reads better than a
	// placeholder and there is no reason to discard it when Google already
	// handed it over. Fall back to the email's local part / a neutral
	// placeholder ONLY when a claim is missing — never merchant-flavoured
	// wording like "Member" on what may be a shopper account. The account
	// is usable for sign-in immediately either way; the identity's own
	// holder can correct their display name afterward like any other
	// profile field. Bounded defensively: a provider payload is untrusted
	// input, even for a field this package never makes a trust decision on.
	localPart := identity.Email
	if i := strings.IndexByte(localPart, '@'); i > 0 {
		localPart = localPart[:i]
	}
	givenName := boundedProfileName(identity.GivenName, localPart)
	familyName := boundedProfileName(identity.FamilyName, "User")

	body := map[string]any{
		"profile": map[string]any{
			"givenName":  givenName,
			"familyName": familyName,
		},
		"email": map[string]any{
			"email":      identity.Email,
			"isVerified": true,
		},
		"idpLinks": []any{
			map[string]any{
				"idpId":    identity.IDPID,
				"userId":   identity.ExternalUserID,
				"userName": identity.ExternalUserName,
			},
		},
	}
	var wire struct {
		UserID string `json:"userId"`
	}
	// withBadRequestErrorForCode(6, ...): grpc code 6 is ALREADY_EXISTS —
	// the specific case where AddHumanUser 400s because the email is
	// already taken (typically the same address in a different case than a
	// case-insensitive FindUserByVerifiedEmail search already ruled out,
	// but also possible on a genuine race — see idpFinish's handling of
	// ErrEmailAlreadyExists). Narrowed to that one code, not every 400: a
	// malformed profile field or any other validation/policy rejection
	// must NOT be mislabelled "email already exists" — that reads as a
	// retryable race to a caller when it is neither. See badRequestCode's
	// doc on requestOptions.
	if err := c.do(ctx, http.MethodPost, "/v2/users/human", body, &wire, ErrUnavailable, withBadRequestErrorForCode(6, ErrEmailAlreadyExists)); err != nil {
		return "", err
	}
	if wire.UserID == "" {
		return "", fmt.Errorf("zitadellogin: create human user: 200 without a userId: %w", ErrUnavailable)
	}
	return wire.UserID, nil
}

// maxProfileNameLength bounds a human profile name field defensively. This
// package makes no trust decision on given_name/family_name (unlike
// email/email_verified), but a provider payload is still untrusted input —
// an unbounded value should not flow into a Zitadel API call unexamined.
const maxProfileNameLength = 200

// boundedProfileName returns raw, truncated to maxProfileNameLength, or
// fallback when raw is empty.
func boundedProfileName(raw, fallback string) string {
	if raw == "" {
		return fallback
	}
	if len(raw) > maxProfileNameLength {
		return raw[:maxProfileNameLength]
	}
	return raw
}

// CreateHumanUserWithPassword registers a brand-new Zitadel human user with
// a password credential, for the storefront customer sign-up path (spec
// D-sign-up-verifies-email in
// .superpowers/sdd/2026-09-04-zitadel-phase6a-customer-signup/progress.md).
// Modelled on CreateHumanUserWithIDPLink: same guard-and-build shape, same
// error-mapping convention, same withLogPath discipline — but there is no
// identity to pin here (this is a first-party credential, not a federated
// one), and the email is NOT marked verified up front. Instead the call
// asks Zitadel for a return code (email.returnCode) that Zitadel hands back
// as emailCode rather than mailing itself, exactly like the password-reset
// flow in zitadeladmin's SendPasswordResetOobCode: this package's own mail
// path (auth-bff's internal/notify) sends the code, which is how the
// account's OWN branded verification email gets sent instead of Zitadel's
// unbranded default.
//
// Callers MUST have already checked FindUserByVerifiedEmail before calling
// this — see idpFinish's identical discipline for CreateHumanUserWithIDPLink
// — and MUST NEVER log or return emailCode to the browser: it is a live
// verification credential for the account just created.
//
// VERIFIED 2026-09-04 against the live TESSERIX Zitadel instance (see this
// package's README and the sdd progress ledger above):
//
//	POST /v2/users/human
//	{"profile":{...},"email":{"email":"...","returnCode":{}},"password":{"password":"..."}}
//	  -> 200 {"userId":..., "details":..., "emailCode":"<6 chars>"}
//
// returnCode sits DIRECTLY under email — protojson flattens oneofs, there is
// no wrapper key named after the oneof itself. Phase 5 shipped a critical
// defect wrapping exactly this shape on password_reset (see
// zitadeladmin's package doc): the wrapped form still returns 200 but
// Zitadel treats it as "no return medium requested", mails the user itself,
// and hands the caller nothing to send. Do not reintroduce that shape here.
func (c *Client) CreateHumanUserWithPassword(ctx context.Context, email, password, givenName, familyName string) (userID, emailCode string, err error) {
	if email == "" {
		return "", "", fmt.Errorf("zitadellogin: refusing to create a user with no email: %w", ErrUnavailable)
	}
	if password == "" {
		return "", "", fmt.Errorf("zitadellogin: refusing to create a user with no password: %w", ErrUnavailable)
	}

	// Same fallback shape as CreateHumanUserWithIDPLink: Zitadel requires
	// non-empty given/family names on a human profile, and a real
	// caller-supplied name reads better than a placeholder — see that
	// function's doc for why the fallback is the email's local part /
	// "User", never a merchant-flavoured word like "Member".
	localPart := email
	if i := strings.IndexByte(localPart, '@'); i > 0 {
		localPart = localPart[:i]
	}
	givenName = boundedProfileName(givenName, localPart)
	familyName = boundedProfileName(familyName, "User")

	body := map[string]any{
		"profile": map[string]any{
			"givenName":  givenName,
			"familyName": familyName,
		},
		"email": map[string]any{
			"email":      email,
			"returnCode": map[string]any{},
		},
		"password": map[string]any{
			"password": password,
		},
	}
	var wire struct {
		UserID    string `json:"userId"`
		EmailCode string `json:"emailCode"`
	}
	// classify distinguishes the two 400 cases AddHumanUser can answer here,
	// which do NOT share a grpc code: a too-short password (code 3, id
	// weakPasswordErrorID) and a duplicate email (code 6, id
	// duplicateEmailErrorID). id is checked FIRST for both — it is the
	// stable, specific signal — with code 6 kept only as a fallback for an
	// ALREADY_EXISTS this package has not seen an id for yet. Anything else
	// falls back to ErrUnavailable rather than guessing — see
	// badRequestClassifier's doc.
	classify := func(code int, id string) error {
		switch {
		case id == weakPasswordErrorID:
			return ErrWeakPassword
		case id == duplicateEmailErrorID:
			return ErrEmailAlreadyExists
		case code == 6:
			return ErrEmailAlreadyExists
		default:
			return ErrUnavailable
		}
	}
	if err := c.do(ctx, http.MethodPost, "/v2/users/human", body, &wire, ErrUnavailable, withBadRequestClassifier(classify)); err != nil {
		return "", "", err
	}
	if wire.UserID == "" {
		return "", "", fmt.Errorf("zitadellogin: create human user with password: 200 without a userId: %w", ErrUnavailable)
	}
	if wire.EmailCode == "" {
		// A 200 with no emailCode means returnCode was not honoured as a
		// return medium (see the wrapped-oneof defect this doc warns
		// about) — Zitadel has already mailed the account itself with a
		// code this package can never deliver through its own branded
		// path. Refuse rather than report success with nothing to send.
		return "", "", fmt.Errorf("zitadellogin: create human user with password: 200 without an emailCode: %w", ErrUnavailable)
	}
	return wire.UserID, wire.EmailCode, nil
}

// LinkIDPToUser attaches the given federated identity to an EXISTING
// Zitadel user, so the very next retrieve of the same provider identity
// resolves IDPIdentity.ZitadelUserID to userID directly.
//
// Guarded exactly like CreateHumanUserWithIDPLink, and for the identical
// reason: linking an unverified provider email to an existing account is
// account takeover. This refuses outright unless identity carries a
// non-empty, verified email. Callers must ALSO have already resolved userID
// themselves (e.g. via FindUserByVerifiedEmail) — this function does not
// look anyone up, it only attaches.
//
// Verified 2026-09-04 against the live TESSERIX Zitadel instance:
// POST /v2/users/{userId}/links with body {"idpLink":{"idpId","userId",
// "userName"}} returns 200 and the link is confirmed to attach via
// POST /v2/users/{userId}/links/_search afterwards.
func (c *Client) LinkIDPToUser(ctx context.Context, userID string, identity IDPIdentity) error {
	if identity.Email == "" || !identity.EmailVerified {
		return fmt.Errorf("zitadellogin: refusing to link an empty or unverified email: %w", ErrUnavailable)
	}
	body := map[string]any{
		"idpLink": map[string]any{
			"idpId":    identity.IDPID,
			"userId":   identity.ExternalUserID,
			"userName": identity.ExternalUserName,
		},
	}
	return c.do(ctx, http.MethodPost, "/v2/users/"+url.PathEscape(userID)+"/links", body, nil, ErrUnavailable)
}

// VerifyTOTP submits a TOTP code.
//
// Two facts, both observed and both easy to get wrong:
//
//  1. The method is PATCH. POST to a session id returns 405.
//  2. The session token ROTATES on every check. The response carries a NEW
//     sessionToken, and finalize needs the newest one. Returning the input
//     session keeps the stale token and makes finalize fail AFTER a correct
//     code, which the user reads as "my code was wrong".
func (c *Client) VerifyTOTP(ctx context.Context, s Session, code string) (Session, error) {
	body := map[string]any{
		"sessionToken": s.Token,
		"checks":       map[string]any{"totp": map[string]any{"code": code}},
	}
	var wire struct {
		SessionToken string `json:"sessionToken"`
	}
	if err := c.do(ctx, http.MethodPatch, "/v2/sessions/"+url.PathEscape(s.ID), body, &wire, ErrBadCredentials); err != nil {
		return Session{}, err
	}
	return Session{ID: s.ID, Token: wire.SessionToken}, nil
}

// SessionFactors re-reads what Zitadel believes was verified. The create
// response omits the factors object entirely, so it cannot be trusted to say
// what was checked — this is the only honest source.
func (c *Client) SessionFactors(ctx context.Context, sessionID string) (Factors, error) {
	var wire struct {
		Session struct {
			Factors struct {
				User *struct {
					ID             string `json:"id"`
					OrganizationID string `json:"organizationId"`
				} `json:"user"`
				Password *struct {
					VerifiedAt string `json:"verifiedAt"`
				} `json:"password"`
				TOTP *struct {
					VerifiedAt string `json:"verifiedAt"`
				} `json:"totp"`
			} `json:"factors"`
		} `json:"session"`
	}
	if err := c.do(ctx, http.MethodGet, "/v2/sessions/"+url.PathEscape(sessionID), nil, &wire, ErrUnavailable); err != nil {
		return Factors{}, err
	}
	f := wire.Session.Factors
	out := Factors{Password: f.Password != nil, TOTP: f.TOTP != nil}
	if f.User != nil {
		out.UserID, out.OrgID = f.User.ID, f.User.OrganizationID
	}
	return out, nil
}

// EnrolledMethodTypes lists a user's registered authentication methods.
func (c *Client) EnrolledMethodTypes(ctx context.Context, userID string) ([]string, error) {
	var wire struct {
		AuthMethodTypes []string `json:"authMethodTypes"`
	}
	if err := c.do(ctx, http.MethodGet, "/v2/users/"+url.PathEscape(userID)+"/authentication_methods", nil, &wire, ErrUnavailable); err != nil {
		return nil, err
	}
	return wire.AuthMethodTypes, nil
}

// UserEmail fetches a user's email address by user id.
//
// This is the ONLY email the handler should trust for a Zitadel-authenticated
// session: it comes from Zitadel's own record of who the session's factors
// belong to, not from anything the caller typed into a request body. Reading
// it here — rather than trusting a client-supplied login_name on the TOTP
// step — is what makes a spoofed login_name on /zitadel/totp inert.
func (c *Client) UserEmail(ctx context.Context, userID string) (string, error) {
	var wire struct {
		User struct {
			Human *struct {
				Email struct {
					Email string `json:"email"`
				} `json:"email"`
			} `json:"human"`
		} `json:"user"`
	}
	if err := c.do(ctx, http.MethodGet, "/v2/users/"+url.PathEscape(userID), nil, &wire, ErrUserNotFound); err != nil {
		return "", err
	}
	if wire.User.Human == nil {
		return "", fmt.Errorf("zitadellogin: user has no human profile, cannot resolve email: %w", ErrUnavailable)
	}
	return wire.User.Human.Email.Email, nil
}

// UserEmailVerified reports whether Zitadel currently marks userID's own
// email verified. Used by the storefront customer password login gate
// (customer_handler.go's login) to refuse a sign-up account that has never
// completed the returnCode flow CreateHumanUserWithPassword started — see
// that function's doc and the sdd progress ledger's
// "sign-up VERIFIES the email" ruling. Without this gate, an attacker could
// register a victim's address with a password of their own choosing and
// sign straight in, which is the exact lockout self-registration with email
// verification exists to prevent.
func (c *Client) UserEmailVerified(ctx context.Context, userID string) (bool, error) {
	var wire struct {
		User struct {
			Human *struct {
				Email struct {
					IsVerified bool `json:"isVerified"`
				} `json:"email"`
			} `json:"human"`
		} `json:"user"`
	}
	if err := c.do(ctx, http.MethodGet, "/v2/users/"+url.PathEscape(userID), nil, &wire, ErrUserNotFound); err != nil {
		return false, err
	}
	if wire.User.Human == nil {
		return false, fmt.Errorf("zitadellogin: user has no human profile, cannot resolve email verification: %w", ErrUnavailable)
	}
	return wire.User.Human.Email.IsVerified, nil
}

// DeleteUser permanently removes the Zitadel user identified by userID.
//
// This exists for exactly one caller today: register's rollback when the
// verification email cannot be sent (see customer_handler.go). An account
// CreateHumanUserWithPassword just created, that this same request has
// never reported success for, is not reachable by anyone else yet — an
// unverified email means no sign-in gate above will admit it (see
// UserEmailVerified's doc), and the storefront never saw a response
// carrying its uid. Deleting it turns a permanent stranded-account lockout
// (see ErrEmailAlreadyExists's doc: a future registration attempt for the
// same address would otherwise 400 forever) into a clean retry.
//
// Idempotent: ErrUserNotFound (a 404) is treated as success, mirroring
// zitadeladmin.Client.DeleteAccount's identical reasoning — a caller
// retrying its own rollback may find the user already gone.
func (c *Client) DeleteUser(ctx context.Context, userID string) error {
	if userID == "" {
		return fmt.Errorf("zitadellogin: DeleteUser with an empty user id: %w", ErrUnavailable)
	}
	err := c.do(ctx, http.MethodDelete, "/v2/users/"+url.PathEscape(userID), nil, nil, ErrUserNotFound, withLogPath("/v2/users/{id}"))
	if errors.Is(err, ErrUserNotFound) {
		return nil
	}
	return err
}

var mfaPolicyKeys = []string{"forceMfa", "forceMfaLocalOnly"}

// policyAnchorKey proves a 200 really carried a login-policy object.
//
// It cannot be one of the MFA booleans: protojson elides zero-value fields, so
// a healthy org that does not force MFA sends no forceMfa key at all. Treating
// that absence as "unrecognized" handed every ordinary login to the hosted UI
// in hms. passwordCheckLifetime is a message-typed field observed present on
// every real response.
const policyAnchorKey = "passwordCheckLifetime"

func (c *Client) loginPolicy(ctx context.Context, opts ...requestOption) (LoginPolicy, error) {
	var wire struct {
		Policy map[string]any `json:"policy"`
	}
	if err := c.do(ctx, http.MethodGet, "/management/v1/policies/login", nil, &wire, ErrUnavailable, opts...); err != nil {
		return LoginPolicy{}, err
	}
	if _, ok := wire.Policy[policyAnchorKey]; !ok {
		return LoginPolicy{}, fmt.Errorf("zitadellogin: 200 without a recognizable policy object: %w", ErrUnavailable)
	}
	for _, key := range mfaPolicyKeys {
		if err := refuseIfKeyRenamedOrRecased(wire.Policy, key); err != nil {
			return LoginPolicy{}, err
		}
	}
	forceMFA, err := readMFABool(wire.Policy, "forceMfa")
	if err != nil {
		return LoginPolicy{}, err
	}
	localOnly, err := readMFABool(wire.Policy, "forceMfaLocalOnly")
	if err != nil {
		return LoginPolicy{}, err
	}
	return LoginPolicy{ForceMFA: forceMFA, ForceMFALocalOnly: localOnly}, nil
}

func normalizePolicyKey(key string) string {
	return strings.ReplaceAll(strings.ToLower(key), "_", "")
}

// refuseIfKeyRenamedOrRecased fails closed when the API renames or re-cases an
// MFA field. Without it, force_mfa or ForceMFA would decode as merely absent,
// therefore false — a silent fail-open on the one field that matters most.
func refuseIfKeyRenamedOrRecased(policy map[string]any, wantKey string) error {
	want := normalizePolicyKey(wantKey)
	for k := range policy {
		if k != wantKey && normalizePolicyKey(k) == want {
			return fmt.Errorf("zitadellogin: policy key %q looks like a renamed %q: %w", k, wantKey, ErrUnavailable)
		}
	}
	return nil
}

func readMFABool(policy map[string]any, key string) (bool, error) {
	v, ok := policy[key]
	if !ok {
		return false, nil // elided zero value: genuinely false
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("zitadellogin: policy.%s is %T, not a bool: %w", key, v, ErrUnavailable)
	}
	return b, nil
}

// LoginPolicyForOrg reads the policy of the org the user actually belongs to.
// It refuses an empty org id rather than falling back to an unscoped read: an
// unscoped read judges a user in one org by a different org's policy, which is
// a real, shipped MFA bypass (hms #913).
func (c *Client) LoginPolicyForOrg(ctx context.Context, orgID string) (LoginPolicy, error) {
	if orgID == "" {
		return LoginPolicy{}, fmt.Errorf("zitadellogin: LoginPolicyForOrg with an empty org id, refusing rather than reading unscoped: %w", ErrUnavailable)
	}
	return c.loginPolicy(ctx, withOrgID(orgID))
}

// InstanceLoginPolicyForDisplay reads the unscoped instance policy. It is for
// display before a user is known and MUST NOT be used for enforcement; an
// archtest forbids sufficiency.go from referencing it.
func (c *Client) InstanceLoginPolicyForDisplay(ctx context.Context) (LoginPolicy, error) {
	return c.loginPolicy(ctx)
}
