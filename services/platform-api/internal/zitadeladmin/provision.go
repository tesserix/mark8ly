package zitadeladmin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/mark8ly/platform-api/internal/gipadmin"
)

// This file holds the PROVISIONING half of the Zitadel admin client:
// creating an invited teammate's account and granting them the admin
// project role. It is separate from client.go's three account
// operations (password-reset send/confirm, delete) because it has a
// different caller (internal/invitation) and a different failure
// posture — see EnsureProjectGrant's doc.
//
// Every request shape below was VERIFIED against the live TESSERIX
// Zitadel instance on 2026-09-04/05 while hand-provisioning the first
// invited staff member; the exceptions are called out inline.
//
// The protojson rules recorded in this package's doc comment apply
// here too and are load-bearing:
//
//   - Oneofs are FLATTENED. Nothing below wraps a variant in a key
//     named after its oneof.
//   - Zero values are ELIDED. "isVerified": true must be sent
//     explicitly; omitting it (or sending the field's zero value)
//     produces an UNVERIFIED email, and an unverified email makes the
//     user invisible to resolveUserIDByEmail, which requires
//     human.email.isVerified. That would turn a successful create into
//     a user nobody can ever look up again.

// zitadelErrIDUserAlreadyExists is the stable details[0].id Zitadel
// returns from POST /v2/users/human when a user with that email
// already exists in the org. VERIFIED live: it arrives as an HTTP 409
// whose grpc code is 6 (AlreadyExists).
//
// classifyError maps that 409 to gipadmin.ErrUnavailable (its default
// branch — "a wrong guess here reads as something it is not"), which
// is the right default for the account operations but useless here:
// "already exists" is the single most common outcome of re-accepting
// an invitation, and it must resolve to the existing user rather than
// abort the accept. Hence the id check rather than a status check.
const zitadelErrIDUserAlreadyExists = "V3-DKcYh"

// ResolveUserIDByEmail is the exported form of resolveUserIDByEmail.
// Callers outside this package (internal/invitation, via the wiring in
// cmd/server) need the same email -> user id resolution the password
// reset path uses, with the same guarantees: scoped to Config.OrgID,
// requires a VERIFIED email, refuses an ambiguous match rather than
// picking one (ErrAmbiguousEmail), and returns gipadmin.ErrUserNotFound
// on zero matches.
func (c *Client) ResolveUserIDByEmail(ctx context.Context, email string) (string, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return "", fmt.Errorf("zitadeladmin: email is required")
	}
	return c.resolveUserIDByEmail(ctx, email)
}

// HumanUser is the input to EnsureHumanUser.
type HumanUser struct {
	Email string
	// FirstName / LastName populate Zitadel's required profile block.
	// Callers that have no real name should pass something derived
	// from the email rather than empty strings — Zitadel rejects an
	// empty givenName/familyName.
	FirstName string
	LastName  string
	// Password is set with changeRequired=false so the invitee can
	// sign in immediately with what they just typed. Required when the
	// user does not already exist.
	Password string
}

// EnsureHumanUser creates the invitee's Zitadel account and returns its
// user id, or returns the id of the account that already exists for
// that email.
//
// The request body is the shape VERIFIED working in production:
//
//	POST /v2/users/human
//	{"organization":{"orgId":…},
//	 "profile":{"givenName":…,"familyName":…},
//	 "email":{"email":…,"isVerified":true},
//	 "password":{"password":…,"changeRequired":false}}
//
// isVerified is true deliberately and is NOT a shortcut: the invitation
// token that got the caller here was emailed to this exact address and
// is single-use, so possession of it IS proof of control of the
// mailbox — the same attestation a verification email would produce.
// It also has to be true for the account to be findable again:
// resolveUserIDByEmail (and zitadellogin.FindUserByVerifiedEmail in
// auth-bff) both require human.email.isVerified, so an unverified
// invitee would be created once and then be unresolvable forever.
//
// "Already exists" is resolved, not failed. Re-accepting an invitation,
// accepting a second invitation to a different tenant with the same
// address, or a retry after a partial failure all land here, and in
// every one of those cases the correct outcome is "use the account that
// is already there" — the caller still needs its id to write the grant
// and the FGA tuples. The supplied Password is deliberately ignored on
// that branch: silently resetting an existing merchant's password from
// an invitation-accept form would be an account takeover primitive.
func (c *Client) EnsureHumanUser(ctx context.Context, in HumanUser) (string, error) {
	email := strings.TrimSpace(in.Email)
	if email == "" {
		return "", fmt.Errorf("zitadeladmin: email is required")
	}
	first := strings.TrimSpace(in.FirstName)
	last := strings.TrimSpace(in.LastName)
	if first == "" || last == "" {
		return "", fmt.Errorf("zitadeladmin: first and last name are required to create a human user")
	}
	if in.Password == "" {
		return "", fmt.Errorf("zitadeladmin: password is required to create a human user")
	}

	body := map[string]any{
		"organization": map[string]any{"orgId": c.orgID},
		"profile": map[string]any{
			"givenName":  first,
			"familyName": last,
		},
		"email": map[string]any{
			"email":      email,
			"isVerified": true,
		},
		"password": map[string]any{
			"password":       in.Password,
			"changeRequired": false,
		},
	}
	var wire struct {
		UserID string `json:"userId"`
	}
	// The org is carried in the BODY here (organization.orgId), which is
	// what the verified-working request does; the org header is not sent
	// so the two cannot disagree about which org the user lands in.
	err := c.do(ctx, http.MethodPost, "/v2/users/human", body, &wire, false, withLogPath("/v2/users/human"))
	if err == nil {
		if wire.UserID == "" {
			return "", fmt.Errorf("zitadeladmin: create human user returned 2xx without a userId: %w", gipadmin.ErrUnavailable)
		}
		return wire.UserID, nil
	}
	if errorID(err) != zitadelErrIDUserAlreadyExists {
		return "", err
	}
	// The account is already there — look it up and carry on.
	existing, resolveErr := c.resolveUserIDByEmail(ctx, email)
	if resolveErr != nil {
		return "", fmt.Errorf("zitadeladmin: user already exists but could not be resolved: %w", resolveErr)
	}
	return existing, nil
}

// EnsureProjectGrant gives userID the given roles on projectID, and
// treats an already-present grant as success.
//
//	POST /management/v1/users/{userId}/grants
//	{"projectId":…,"roleKeys":[…]}   + the x-zitadel-orgid header
//
// This call is not optional decoration. The mark8ly-admin project has
// projectRoleCheck: true (see the zitadel-bootstrap chart values), so a
// user holding NO role on it cannot complete the OIDC flow at all —
// finalize returns 403 OIDC-foSyH49RvL. An invited teammate with FGA
// tuples but no project grant is an account that looks fully created
// and can never sign in.
//
// Idempotency is by tolerating the duplicate rather than by searching
// first: an extra list call would be a second, UNVERIFIED request shape
// on the critical path, and a re-accept must not fail merely because
// the grant survived from the first attempt. The duplicate is detected
// by grpc code 6 (AlreadyExists) / HTTP 409, with a message-substring
// safety net. Unlike the create case above there is no live-verified
// error id for this one, which is exactly why the detection is
// deliberately broad here and narrow (id-keyed) there.
func (c *Client) EnsureProjectGrant(ctx context.Context, userID, projectID string, roleKeys []string) error {
	userID = strings.TrimSpace(userID)
	projectID = strings.TrimSpace(projectID)
	if userID == "" {
		return fmt.Errorf("zitadeladmin: userID is required")
	}
	if projectID == "" {
		return fmt.Errorf("zitadeladmin: projectID is required")
	}
	if len(roleKeys) == 0 {
		return fmt.Errorf("zitadeladmin: at least one role key is required")
	}

	body := map[string]any{
		"projectId": projectID,
		"roleKeys":  roleKeys,
	}
	path := fmt.Sprintf("/management/v1/users/%s/grants", url.PathEscape(userID))
	err := c.do(ctx, http.MethodPost, path, body, nil, true, withLogPath("/management/v1/users/{id}/grants"))
	if err == nil || isAlreadyExists(err) {
		return nil
	}
	return err
}

// isAlreadyExists reports whether err is Zitadel refusing to create
// something that is already there. See EnsureProjectGrant's doc for why
// this is broader than the id-keyed check EnsureHumanUser uses.
func isAlreadyExists(err error) bool {
	var ae *apiError
	if !errors.As(err, &ae) {
		return false
	}
	if ae.grpcCode == 6 || ae.status == http.StatusConflict {
		return true
	}
	// Both spellings occur in Zitadel's own strings: the human message
	// ("User grant already exists") and the i18n key it sometimes
	// surfaces instead ("Errors.UserGrant.AlreadyExists").
	msg := strings.ToLower(ae.message)
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "alreadyexists")
}

// StaffProvisioner binds a Client to the admin project and role key an
// invited teammate must hold, and performs the whole provisioning step
// as one call. It satisfies internal/invitation's StaffProvisioner
// interface; the invitation package deliberately knows nothing about
// Zitadel projects or role keys, which are deployment config.
type StaffProvisioner struct {
	client    *Client
	projectID string
	roleKeys  []string
}

// NewStaffProvisioner constructs a StaffProvisioner. It refuses an
// empty project id or role set rather than provisioning users that
// cannot sign in — see EnsureProjectGrant's doc.
func NewStaffProvisioner(client *Client, projectID string, roleKeys []string) (*StaffProvisioner, error) {
	if client == nil {
		return nil, fmt.Errorf("zitadeladmin: client is required")
	}
	if strings.TrimSpace(projectID) == "" {
		return nil, fmt.Errorf("zitadeladmin: admin project id is required")
	}
	if len(roleKeys) == 0 {
		return nil, fmt.Errorf("zitadeladmin: at least one staff role key is required")
	}
	return &StaffProvisioner{
		client:    client,
		projectID: strings.TrimSpace(projectID),
		roleKeys:  roleKeys,
	}, nil
}

// ProvisionStaff makes email a sign-in-capable identity on the admin
// project and returns its Zitadel user id.
//
// Resolve-first, create-on-miss: an address that already has a Zitadel
// account (a merchant accepting a second invitation, or anyone
// re-accepting after a partial failure) needs no password at all, and
// asking EnsureHumanUser to create it would only reach the same
// resolution via a wasted 409. password is therefore only consulted
// when the account genuinely does not exist yet. EnsureHumanUser's own
// already-exists branch remains the backstop for the race between this
// lookup and the create.
//
// The grant is ensured on EVERY call, including for a pre-existing
// user: an account created by some other path (storefront signup, an
// earlier GIP-era migration) has no grant on the admin project, and
// without one it cannot complete the OIDC flow.
func (p *StaffProvisioner) ProvisionStaff(ctx context.Context, email, firstName, lastName, password string) (string, error) {
	userID, err := p.client.ResolveUserIDByEmail(ctx, email)
	switch {
	case err == nil:
		// Existing account — password intentionally unused.
	case errors.Is(err, gipadmin.ErrUserNotFound):
		userID, err = p.client.EnsureHumanUser(ctx, HumanUser{
			Email:     email,
			FirstName: firstName,
			LastName:  lastName,
			Password:  password,
		})
		if err != nil {
			return "", err
		}
	default:
		// Includes ErrAmbiguousEmail: two accounts share this address,
		// and picking one to grant admin access to is not a decision to
		// make silently.
		return "", err
	}

	if err := p.client.EnsureProjectGrant(ctx, userID, p.projectID, p.roleKeys); err != nil {
		return "", err
	}
	return userID, nil
}
