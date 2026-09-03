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
)

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
}

type requestOption func(*requestOptions)

func withOrgID(orgID string) requestOption {
	return func(ro *requestOptions) { ro.orgID = orgID }
}

func (c *Client) do(ctx context.Context, method, path string, body, out any, notFound error, opts ...requestOption) error {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("zitadellogin: marshal %s %s: %w", method, path, err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return fmt.Errorf("zitadellogin: build %s %s: %w", method, path, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	var ro requestOptions
	for _, opt := range opts {
		opt(&ro)
	}
	if ro.orgID != "" {
		req.Header.Set("x-zitadel-orgid", ro.orgID)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("zitadellogin: %s %s: %v: %w", method, path, err, ErrUnavailable)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		id := readZitadelErrorID(resp.Body)
		switch {
		case resp.StatusCode == http.StatusBadRequest:
			return fmt.Errorf("zitadellogin: %s %s: %s: %w", method, path, id, ErrBadCredentials)
		case resp.StatusCode == http.StatusNotFound:
			return fmt.Errorf("zitadellogin: %s %s: %s: %w", method, path, id, notFound)
		default:
			return fmt.Errorf("zitadellogin: %s %s: status %d: %s: %w", method, path, resp.StatusCode, id, ErrUnavailable)
		}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxSuccessBodyBytes)).Decode(out); err != nil {
		return fmt.Errorf("zitadellogin: decode %s %s: %v: %w", method, path, err, ErrUnavailable)
	}
	return nil
}

// readZitadelErrorID extracts ONLY details[0].id (e.g. "COMMAND-3M0fs"). The
// raw error body is never surfaced, because that is exactly where Zitadel puts
// failedAttempts — a counter that must not reach a caller or a log line.
func readZitadelErrorID(r io.Reader) string {
	var wire struct {
		Details []struct {
			ID string `json:"id"`
		} `json:"details"`
	}
	if err := json.NewDecoder(io.LimitReader(r, maxErrorBodyBytes)).Decode(&wire); err != nil {
		return "unknown"
	}
	if len(wire.Details) == 0 || wire.Details[0].ID == "" {
		return "unknown"
	}
	return wire.Details[0].ID
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

// FindUserByVerifiedEmail searches for an EXISTING Zitadel user whose email
// matches exactly and whose email Zitadel itself has separately marked
// verified. Returns "" with a nil error when no such user exists — an
// ordinary, expected outcome for a first-time federated identity, not a
// failure.
//
// This exists to stop a brand-new federated sign-in from creating a
// SECOND, disconnected account for someone who already has one: without
// it, CreateHumanUserWithIDPLink would either fail on a duplicate-email
// conflict or (worse, if Zitadel's own uniqueness constraint were ever
// relaxed) silently split one person into two identities.
//
// NOT directly observed against a live instance with the v2 ListUsers
// endpoint at the time this was written — modelled on Zitadel's documented
// v2 UserService.ListUsers ("POST /v2/users") request/response shape: a
// queries array carrying an emailQuery, and a "result" array of users each
// optionally carrying a human profile. See this package's README for the
// "observed vs documented" convention, and idpFinish's doc comment for why
// this being unverified against a live instance matters here specifically.
func (c *Client) FindUserByVerifiedEmail(ctx context.Context, email string) (string, error) {
	body := map[string]any{
		"queries": []any{
			map[string]any{
				"emailQuery": map[string]any{
					"emailAddress": email,
					"method":       "TEXT_QUERY_METHOD_EQUALS",
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
	if err := c.do(ctx, http.MethodPost, "/v2/users", body, &wire, ErrUnavailable); err != nil {
		return "", err
	}
	for _, u := range wire.Result {
		if u.Human != nil && u.Human.Email.Email == email && u.Human.Email.IsVerified {
			return u.UserID, nil
		}
	}
	return "", nil
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

	// Zitadel requires non-empty given/family names on a human profile. The
	// federated identity carries no name split reliably (rawInformation's
	// shape is provider-defined and only email/email_verified are read
	// elsewhere in this package — see readRawEmail), so this uses the
	// email's local part as a functional placeholder. It makes the account
	// usable for sign-in immediately; a merchant can correct the display
	// name afterward like any other profile field.
	localPart := identity.Email
	if i := strings.IndexByte(localPart, '@'); i > 0 {
		localPart = localPart[:i]
	}

	body := map[string]any{
		"profile": map[string]any{
			"givenName":  localPart,
			"familyName": "Member",
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
	if err := c.do(ctx, http.MethodPost, "/v2/users/human", body, &wire, ErrUnavailable); err != nil {
		return "", err
	}
	if wire.UserID == "" {
		return "", fmt.Errorf("zitadellogin: create human user: 200 without a userId: %w", ErrUnavailable)
	}
	return wire.UserID, nil
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
