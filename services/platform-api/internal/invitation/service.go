package invitation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/mark8ly/platform-api/internal/audit"
	"github.com/mark8ly/platform-api/internal/authz"
	"github.com/mark8ly/platform-api/internal/notification"
	"github.com/mark8ly/platform-api/internal/store"
	"github.com/mark8ly/platform-api/internal/tenant"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Service is the business-logic layer for invitations.
//
// It orchestrates: the FGA permission check, the token generation,
// the DB row, the email send, and — on accept — the FGA role write.
// Keeping all of it in one file is deliberate; the invitation lifecycle
// is small enough that the handler/service/repo split is the only
// layering that matters.
type Service struct {
	repo        Repository
	tenantRepo  tenant.Repository
	storeRepo   store.Repository
	fga         authz.Client
	sender      notification.Sender
	loader      *notification.Loader
	emailFrom   string
	acceptURL   func(slug, token string) string
	expiry      time.Duration
	recorder    TokenRecorder
	audit       *audit.Client // optional — emits staff lifecycle events
	provisioner StaffProvisioner
}

// StaffProvisioner makes an invited teammate a usable sign-in identity
// in the identity provider and returns their provider user id.
//
// Non-nil ONLY on the Zitadel path (see cmd/server/provider_wiring.go's
// newStaffProvisioner). Nil selects the GIP path, whose behaviour is
// unchanged: GIP accounts are created client-side by the accept form
// before it ever calls platform-api, so there is nothing to provision
// server-side and the caller-supplied uid is already a real identity.
//
// Satisfied by *zitadeladmin.StaffProvisioner, which creates-or-
// resolves the Zitadel user AND ensures the mark8ly-admin project
// grant. Both halves are inside one call deliberately: a teammate with
// an account but no grant cannot complete the OIDC flow (the project
// sets projectRoleCheck: true and finalize returns
// 403 OIDC-foSyH49RvL), so "provisioned" is not a state either half
// can reach on its own.
type StaffProvisioner interface {
	ProvisionStaff(ctx context.Context, email, firstName, lastName, password string) (string, error)
}

// TokenRecorder is the narrow interface the service calls to publish
// plaintext tokens to the dev test helper. Kept generic so the
// existing verification.TokenRecorder can't be reused directly (its
// key is email-only and we need tenant+email to disambiguate).
type TokenRecorder interface {
	Record(tenantID, email, token string)
}

// Config holds Service dependencies.
type Config struct {
	Repo       Repository
	TenantRepo tenant.Repository
	// StoreRepo is used to look up the tenant's default store when
	// building the invitation accept URL — phase Q moved the slug
	// (which parameterises the admin subdomain) off the tenant and
	// onto the store. The "default" store is the first one created,
	// i.e. the store that onboarding set up.
	StoreRepo store.Repository
	FGA       authz.Client
	Sender    notification.Sender
	// Loader is the DB-backed template loader (embedded fallback). May
	// be nil; nil falls through to embedded rendering.
	Loader    *notification.Loader
	EmailFrom string
	// AcceptURL builds the full invitation accept URL from the
	// default-store slug and a plaintext token.
	AcceptURL func(slug, token string) string
	// Expiry defaults to 72h when zero.
	Expiry   time.Duration
	Recorder TokenRecorder
	// Audit posts cross-service audit events to marketplace-api.
	// Optional — empty client makes every emit a no-op.
	Audit *audit.Client
	// Provisioner creates the invitee's identity-provider account and
	// project grant on accept. Nil on the GIP path — see
	// StaffProvisioner's doc.
	Provisioner StaffProvisioner
}

// NewService constructs a Service.
func NewService(cfg Config) *Service {
	if cfg.Expiry == 0 {
		cfg.Expiry = 72 * time.Hour
	}
	return &Service{
		repo:        cfg.Repo,
		tenantRepo:  cfg.TenantRepo,
		storeRepo:   cfg.StoreRepo,
		fga:         cfg.FGA,
		sender:      cfg.Sender,
		loader:      cfg.Loader,
		emailFrom:   cfg.EmailFrom,
		acceptURL:   cfg.AcceptURL,
		expiry:      cfg.Expiry,
		recorder:    cfg.Recorder,
		audit:       cfg.Audit,
		provisioner: cfg.Provisioner,
	}
}

// defaultStoreSlug returns the slug of the tenant's first store, or
// the empty string if the tenant has no stores (shouldn't happen
// post-Phase-Q — onboarding creates a default store). Returning the
// empty string falls back to the flat-host accept URL.
func (s *Service) defaultStoreSlug(ctx context.Context, tenantID string) string {
	if s.storeRepo == nil {
		return ""
	}
	stores, err := s.storeRepo.ListByTenant(ctx, tenantID)
	if err != nil || len(stores) == 0 {
		return ""
	}
	return stores[0].Slug
}

// CreateInput is the payload for a new invitation.
//
// Phase R: StoreID is optional. When set, the invitation grants a
// store-level role (manager/staff/viewer) scoped to that specific
// store; when nil, it grants a tenant-level role (admin/staff/viewer)
// over the whole tenant (Phase P behaviour).
type CreateInput struct {
	TenantID        string
	StoreID         string
	Email           string
	Role            string
	InvitedByUserID string
}

// Create writes a new pending invitation, sends the email, and
// returns the row. Runs the FGA `can_invite_members` check against
// the inviting user.
func (s *Service) Create(ctx context.Context, in CreateInput) (*Invitation, error) {
	tenantID := strings.TrimSpace(in.TenantID)
	storeID := strings.TrimSpace(in.StoreID)
	email := strings.ToLower(strings.TrimSpace(in.Email))
	role := strings.TrimSpace(in.Role)
	uid := strings.TrimSpace(in.InvitedByUserID)

	if tenantID == "" {
		return nil, apperrors.BadRequest("invalid_tenant_id", "tenant id is required")
	}
	if email == "" || !strings.Contains(email, "@") {
		return nil, apperrors.BadRequest("invalid_email", "a valid email is required")
	}
	if uid == "" {
		return nil, apperrors.BadRequest("invalid_inviter", "invited_by_user_id is required")
	}
	// Phase R: role allowlist depends on scope. Tenant-wide
	// invitations accept admin/staff/viewer; store-scoped ones
	// accept manager/staff/viewer (matching the store type's
	// DSL relations).
	if storeID == "" {
		if !isAllowedTenantRole(role) {
			return nil, apperrors.BadRequest("invalid_role", "role must be admin, staff, or viewer")
		}
	} else {
		if !isAllowedStoreRole(role) {
			return nil, apperrors.BadRequest("invalid_role", "store role must be manager, staff, or viewer")
		}
	}

	// Authorize the invite. Both tenant-wide and store-scoped
	// invitations are currently gated on the tenant-level
	// can_invite_members permission — a store-scoped invite still
	// requires the inviter to be a tenant owner/admin. Phase R+
	// could introduce a per-store can_invite_store_members if we
	// ever want a store manager to self-service invite their own
	// staff without bothering the tenant owner.
	if s.fga != nil {
		allowed, err := s.fga.Check(ctx, uid, "can_invite_members", tenantID)
		if err != nil {
			return nil, apperrors.Internal("authz_check_failed", "authorization check failed")
		}
		if !allowed {
			return nil, apperrors.New(403, "forbidden", "you do not have permission to invite members")
		}
	}

	// Look up the tenant so we can put its name in the email.
	t, err := s.tenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// If a store is specified, sanity-check it belongs to the
	// target tenant. Stops a malicious admin BFF from minting
	// cross-tenant invitations.
	if storeID != "" && s.storeRepo != nil {
		target, err := s.storeRepo.GetByID(ctx, storeID)
		if err != nil {
			return nil, err
		}
		if target.TenantID != tenantID {
			return nil, apperrors.New(403, "store_tenant_mismatch", "store does not belong to this tenant")
		}
	}

	token, hash, err := newToken()
	if err != nil {
		return nil, apperrors.Wrap(err, 500, "token_generation_failed", "failed to generate invitation token")
	}

	inv := &Invitation{
		TenantID:        tenantID,
		Email:           email,
		Role:            role,
		TokenHash:       hash,
		ExpiresAt:       time.Now().Add(s.expiry),
		Status:          StatusPending,
		InvitedByUserID: uid,
	}
	if storeID != "" {
		sid := storeID
		inv.StoreID = &sid
	}
	if err := s.repo.Create(ctx, inv); err != nil {
		return nil, err
	}

	// Record the plaintext token for the dev e2e helper BEFORE
	// sending — if the sender blows up we still want the e2e to
	// be able to fetch the token.
	if s.recorder != nil {
		s.recorder.Record(tenantID, email, token)
	}

	// Send the email. Non-fatal: a failed send leaves the row
	// pending and the user can retry via "resend" (future slice).
	if s.sender != nil {
		slug := s.defaultStoreSlug(ctx, t.ID)
		url := s.acceptURL(slug, token)
		vars := notification.InvitationVars{
			TenantName:   t.Name,
			Role:         role,
			AcceptURL:    url,
			ExpiresIn:    "72 hours",
			SupportEmail: s.emailFrom,
		}
		// DB-loaded template with embedded fallback. Tenant context is
		// known here so forward it for engagement attribution.
		var (
			msg notification.Email
			err error
		)
		if s.loader != nil {
			msg, err = s.loader.Render(ctx, "invitation", email, s.emailFrom, tenantID, vars)
		} else {
			msg, err = notification.RenderInvitation(email, s.emailFrom, vars)
			if err == nil {
				msg.TenantID = tenantID
			}
		}
		if err == nil {
			_ = s.sender.Send(ctx, msg)
		}
	}

	storeIDForEvent := ""
	if inv.StoreID != nil {
		storeIDForEvent = *inv.StoreID
	}
	s.audit.EmitAsync(audit.Event{
		TenantID:     tenantID,
		StoreID:      storeIDForEvent,
		Action:       "staff.invited",
		ResourceType: "invitation",
		ResourceID:   inv.ID,
		ActorType:    "user",
		ActorUserID:  uid,
		Metadata: map[string]any{
			"invited_email": email,
			"role":          role,
		},
	})

	return inv, nil
}

// Verify looks up an invitation by its plaintext token, validates
// that it's still pending and unexpired, and returns a sanitised
// summary for the public accept page. Never returns the token.
type VerifyResult struct {
	InvitationID string    `json:"invitation_id"`
	TenantID     string    `json:"tenant_id"`
	TenantName   string    `json:"tenant_name"`
	TenantSlug   string    `json:"tenant_slug"`
	Email        string    `json:"email"`
	Role         string    `json:"role"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Verify returns the public view of an invitation or a specific
// error code the accept page surfaces to the user.
func (s *Service) Verify(ctx context.Context, plainToken string) (*VerifyResult, error) {
	plainToken = strings.TrimSpace(plainToken)
	if plainToken == "" {
		return nil, apperrors.BadRequest("invalid_token", "token is required")
	}
	hash := hashToken(plainToken)
	inv, err := s.repo.GetByTokenHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	if inv.Status != StatusPending {
		return nil, apperrors.Conflict("invitation_"+inv.Status, "this invitation is "+inv.Status)
	}
	if time.Now().After(inv.ExpiresAt) {
		return nil, apperrors.Conflict("invitation_expired", "this invitation has expired")
	}
	t, err := s.tenantRepo.GetByID(ctx, inv.TenantID)
	if err != nil {
		return nil, err
	}
	return &VerifyResult{
		InvitationID: inv.ID,
		TenantID:     inv.TenantID,
		TenantName:   t.Name,
		TenantSlug:   s.defaultStoreSlug(ctx, inv.TenantID),
		Email:        inv.Email,
		Role:         inv.Role,
		ExpiresAt:    inv.ExpiresAt,
	}, nil
}

// AcceptInput is the payload for accepting an invitation.
type AcceptInput struct {
	Token string
	// UID is the GIP UID of the accepting user. The caller
	// (admin BFF) has verified the GIP id_token and extracted
	// the uid + verified email.
	//
	// Required on the GIP path. On the Zitadel path it is optional and
	// unused: the invitee has no provider account yet at this point —
	// creating one is what Accept does — so there is no id for the
	// caller to send.
	UID string
	// VerifiedEmail is the GIP token's verified email claim.
	// Must match the invitation's email (case-insensitive).
	//
	// On the Zitadel path the caller sends the address the accept form
	// was opened for; the invitation token itself, single-use and
	// emailed to that address, is what attests to it.
	VerifiedEmail string
	// Password is the password to create the invitee's Zitadel account
	// with. Zitadel path only, and only consulted when no account
	// exists for the address yet. Ignored entirely on the GIP path,
	// where the account already exists before Accept is called.
	Password string
	// FirstName / LastName populate the Zitadel profile. Optional —
	// derived from the email local part when absent (Zitadel rejects
	// an empty givenName/familyName).
	FirstName string
	LastName  string
}

// AcceptResult is returned on successful accept so the admin BFF
// can call /auth/switch-tenant with the new tenant id.
type AcceptResult struct {
	TenantID string `json:"tenant_id"`
	Role     string `json:"role"`
}

// Accept validates the token, writes the FGA role tuple, and marks
// the invitation accepted. Strictly enforces email match to prevent
// bearer-token abuse.
func (s *Service) Accept(ctx context.Context, in AcceptInput) (*AcceptResult, error) {
	token := strings.TrimSpace(in.Token)
	uid := strings.TrimSpace(in.UID)
	verifiedEmail := strings.ToLower(strings.TrimSpace(in.VerifiedEmail))
	if token == "" || verifiedEmail == "" {
		return nil, apperrors.BadRequest("invalid_input", "token, uid, and verified_email are required")
	}
	// uid is required on the GIP path only. With a provisioner wired
	// (Zitadel) the invitee has no provider account yet, so there is no
	// uid to send — this call is what creates one. The message and code
	// are unchanged so the GIP path's response is byte-identical.
	if s.provisioner == nil && uid == "" {
		return nil, apperrors.BadRequest("invalid_input", "token, uid, and verified_email are required")
	}

	inv, err := s.repo.GetByTokenHash(ctx, hashToken(token))
	if err != nil {
		return nil, err
	}
	if inv.Status != StatusPending {
		return nil, apperrors.Conflict("invitation_"+inv.Status, "this invitation is "+inv.Status)
	}
	if time.Now().After(inv.ExpiresAt) {
		return nil, apperrors.Conflict("invitation_expired", "this invitation has expired")
	}

	// Strict email match. Prevents a stolen link from being used
	// by anyone other than the original invitee.
	if !strings.EqualFold(inv.Email, verifiedEmail) {
		return nil, apperrors.New(
			403,
			"email_mismatch",
			"this invitation was sent to a different email address",
		)
	}

	// Provision the invitee in the identity provider (Zitadel path
	// only; s.provisioner is nil under GIP). This runs BEFORE any FGA
	// write and BEFORE MarkAccepted, and a failure aborts the whole
	// accept with nothing written.
	//
	// That ordering is the point. The failure this exists to prevent is
	// a HALF-provisioned teammate — tuples but no project grant, or a
	// grant but no tuples — which is indistinguishable from a working
	// account until they try to sign in and get "we couldn't find a
	// store for this account" or a bare 403 from Zitadel's finalize.
	// Diagnosing one of those took an hour of production archaeology.
	// An invitation that stays pending and returns an error the accept
	// form can show is recoverable by clicking the link again; an
	// account that looks created and cannot sign in is not recoverable
	// by the merchant at all.
	subjects := []string{uid}
	if s.provisioner != nil {
		first, last := profileNames(in.FirstName, in.LastName, inv.Email)
		zitadelUID, err := s.provisioner.ProvisionStaff(ctx, inv.Email, first, last, in.Password)
		if err != nil {
			return nil, provisioningError(err, inv.ID, inv.TenantID, in.Password)
		}
		// Both identity keys, deliberately. Three readers disagree on
		// what a member is keyed by:
		//
		//   - admin sign-in (apps/admin/app/login/actions.ts,
		//     resolveWorkspaceTenant) looks membership up by EMAIL on
		//     the Zitadel path — it has no uid at tenant-resolution
		//     time, which runs BEFORE authentication;
		//   - the bearer-token API path resolves by the Zitadel uid
		//     (the token's `sub`);
		//   - this code used to write only the caller-supplied GIP uid,
		//     which after the Zitadel cutover matches neither.
		//
		// Writing one key breaks the other reader: email-only 403s
		// every API call, uid-only puts the teammate back on "we
		// couldn't find a store for this account" at the login screen.
		// Writing both is what the one working owner account
		// (demo@mark8ly.com) already has, and it needs no change to the
		// login path — which is why it wins over the tidier
		// alternative of teaching every reader one canonical key.
		subjects = []string{inv.Email, zitadelUID}
		// The Zitadel uid, not the caller's, is what gets recorded as
		// the accepting user: it is the id the API path sees and the
		// one UpdateMemberRole later writes tuples for.
		uid = zitadelUID
	}

	// Write the FGA role tuple. Phase R branches on scope:
	// store-scoped invitations write to the store object; tenant-
	// scoped invitations keep the Phase P tenant-level WriteRole.
	//
	// A store-scoped invite ALSO writes a minimal tenant-level
	// `viewer` tuple so auth-bff's autologin (which checks
	// tenant.member) can mint the session cookie. Without this
	// the store-scoped user would be stuck at /login because they
	// have store membership but no tenant membership, and the
	// session-mint path only checks the tenant. The net effect
	// is that a store-scoped invitee can see tenant-level read-
	// only views in addition to their store-level grant — a
	// documented tradeoff for Phase R's minimal footprint.
	//
	// Degraded mode (fga nil) skips — dev-only, never prod.
	if s.fga != nil {
		for _, subject := range subjects {
			if inv.StoreID != nil && *inv.StoreID != "" {
				if err := s.fga.WriteRoleObject(ctx, subject, inv.Role, "store", *inv.StoreID); err != nil {
					return nil, apperrors.Wrap(err, 500, "authz_write_failed", "failed to grant store role")
				}
				// Back-fill a tenant viewer so session mint works.
				if err := s.fga.WriteRole(ctx, subject, authz.RoleViewer, inv.TenantID); err != nil {
					return nil, apperrors.Wrap(err, 500, "authz_write_failed", "failed to grant tenant viewer")
				}
			} else {
				role := authz.Role(inv.Role)
				if err := s.fga.WriteRole(ctx, subject, role, inv.TenantID); err != nil {
					return nil, apperrors.Wrap(err, 500, "authz_write_failed", "failed to grant role")
				}
			}
		}
	}

	if err := s.repo.MarkAccepted(ctx, inv.ID, uid); err != nil {
		return nil, err
	}

	storeIDForEvent := ""
	if inv.StoreID != nil {
		storeIDForEvent = *inv.StoreID
	}
	s.audit.EmitAsync(audit.Event{
		TenantID:     inv.TenantID,
		StoreID:      storeIDForEvent,
		Action:       "staff.invitation_accepted",
		ResourceType: "invitation",
		ResourceID:   inv.ID,
		ActorType:    "user",
		ActorUserID:  uid,
		ActorEmail:   inv.Email,
		Metadata: map[string]any{
			"role":          inv.Role,
			"invited_email": inv.Email,
		},
	})
	return &AcceptResult{
		TenantID: inv.TenantID,
		Role:     inv.Role,
	}, nil
}

// UpdateMemberRoleInput is the request for the role-change endpoint.
type UpdateMemberRoleInput struct {
	TenantID    string
	TargetEmail string
	NewRole     string
	ActorUID    string
}

// UpdateMemberRoleResult is what the handler returns to the caller.
type UpdateMemberRoleResult struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// UpdateMemberRole changes a teammate's tenant-level role. Guarded
// by the actor's own role:
//
//   - Owner: can change any non-self, non-owner member.
//   - Admin: can change only staff/viewer members (cannot touch
//     other admins or the owner).
//   - Anyone else: denied.
//
// The target is identified by email so the admin UI doesn't need to
// know the GIP UID. We look up the target's UID from the invitations
// table (populated at accept time as of migration 0012). Invitations
// accepted before that migration have no UID and therefore cannot be
// role-changed via this endpoint — they have to be revoked and
// re-invited. The service returns a clear error in that case so the
// UI can surface actionable guidance.
func (s *Service) UpdateMemberRole(ctx context.Context, in UpdateMemberRoleInput) (*UpdateMemberRoleResult, error) {
	tenantID := strings.TrimSpace(in.TenantID)
	targetEmail := strings.ToLower(strings.TrimSpace(in.TargetEmail))
	newRole := strings.ToLower(strings.TrimSpace(in.NewRole))
	actorUID := strings.TrimSpace(in.ActorUID)

	if tenantID == "" || targetEmail == "" || newRole == "" || actorUID == "" {
		return nil, apperrors.BadRequest("invalid_input", "tenant_id, email, new_role, and uid are required")
	}
	if !isAllowedTenantRole(newRole) {
		return nil, apperrors.BadRequest("invalid_role", "role must be admin, staff, or viewer")
	}

	// Actor authz: must be owner or admin on this tenant.
	actorRole, err := s.actorRole(ctx, actorUID, tenantID)
	if err != nil {
		return nil, err
	}
	if actorRole != authz.RoleOwner && actorRole != authz.RoleAdmin {
		return nil, apperrors.New(403, "forbidden", "you do not have permission to change team roles")
	}

	// Resolve the target tenant + owner email so we can guard against
	// the "change the owner" footgun without trusting client input.
	t, err := s.tenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(targetEmail, t.OwnerEmail) {
		return nil, apperrors.New(403, "forbidden", "ownership can't be changed here — contact support")
	}

	// Look up the target's UID via their most recent accepted invite.
	inv, err := s.repo.FindAcceptedByEmail(ctx, tenantID, targetEmail)
	if err != nil {
		return nil, err
	}
	if inv.AcceptedByUserID == nil || *inv.AcceptedByUserID == "" {
		return nil, apperrors.New(
			409,
			"uid_not_recorded",
			"this teammate was invited before role editing was supported — please revoke and re-invite them",
		)
	}
	targetUID := *inv.AcceptedByUserID

	// Self-change guard: actors can never change their own role. Keeps
	// the "admin accidentally demotes themselves to viewer" scenario
	// out of the ops inbox.
	if targetUID == actorUID {
		return nil, apperrors.New(403, "forbidden", "you can't change your own role")
	}

	// Admin→admin/owner guard: admins can only touch staff/viewer
	// rows. Owners have free reign over non-owner rows.
	if actorRole == authz.RoleAdmin {
		currentRole := strings.ToLower(inv.Role)
		if currentRole == "admin" || currentRole == "owner" {
			return nil, apperrors.New(403, "forbidden", "admins can only change staff and viewer roles")
		}
		if newRole == "admin" {
			return nil, apperrors.New(403, "forbidden", "only owners can promote members to admin")
		}
	}

	// Write the new FGA tuple. WriteRole is idempotent — if the user
	// already holds the target role this is a no-op. The OLD tuple is
	// left in place for the audit trail; GetRole returns the highest-
	// priority relation so the runtime behaviour reflects the new
	// role immediately.
	if s.fga != nil {
		if err := s.fga.WriteRole(ctx, targetUID, authz.Role(newRole), tenantID); err != nil {
			return nil, apperrors.Wrap(err, 500, "authz_write_failed", "failed to write new role")
		}
		// Zitadel path: Accept wrote BOTH a uid-keyed and an
		// email-keyed tuple because the login path resolves membership
		// by email (see Accept's subjects comment). A role change that
		// updated only the uid-keyed tuple would leave the login path
		// still reading the OLD role — the change would appear to work
		// in the team list and do nothing at sign-in. Kept behind the
		// provisioner nil-check so the GIP path writes exactly the one
		// tuple it always has.
		if s.provisioner != nil {
			if err := s.fga.WriteRole(ctx, targetEmail, authz.Role(newRole), tenantID); err != nil {
				return nil, apperrors.Wrap(err, 500, "authz_write_failed", "failed to write new role")
			}
		}
	}

	// Update the display label on the invitation row so ListMembers
	// reflects the new role without another FGA roundtrip.
	previousRole := strings.ToLower(inv.Role)
	if err := s.repo.UpdateRoleByEmail(ctx, tenantID, targetEmail, newRole); err != nil {
		return nil, err
	}

	storeIDForEvent := ""
	if inv.StoreID != nil {
		storeIDForEvent = *inv.StoreID
	}
	severity := ""
	// Demotions (admin → staff/viewer, staff → viewer) are operationally
	// noisier than promotions and worth flagging as warnings so they
	// stand out in the audit timeline.
	if isDemotion(previousRole, newRole) {
		severity = "warning"
	}
	s.audit.EmitAsync(audit.Event{
		TenantID:     tenantID,
		StoreID:      storeIDForEvent,
		Action:       "staff.role_changed",
		ResourceType: "staff_member",
		ResourceID:   targetUID,
		Severity:     severity,
		ActorType:    "user",
		ActorUserID:  actorUID,
		Metadata: map[string]any{
			"target_email":  targetEmail,
			"previous_role": previousRole,
			"new_role":      newRole,
		},
	})

	return &UpdateMemberRoleResult{Email: targetEmail, Role: newRole}, nil
}

// roleRank gives an ordering used by isDemotion. Higher = more
// privileged. Roles outside the known set return 0 so unknown values
// don't trigger false demotions.
func roleRank(r string) int {
	switch r {
	case "owner":
		return 4
	case "admin":
		return 3
	case "staff":
		return 2
	case "viewer":
		return 1
	default:
		return 0
	}
}

func isDemotion(prev, next string) bool {
	return roleRank(prev) > 0 && roleRank(next) > 0 && roleRank(next) < roleRank(prev)
}

// actorRole is a small helper that resolves the caller's effective
// tenant role via FGA GetRole. Returns ("", error) on failure.
func (s *Service) actorRole(ctx context.Context, uid, tenantID string) (authz.Role, error) {
	if s.fga == nil {
		return "", apperrors.New(503, "authz_unavailable", "authorization service is not available")
	}
	role, err := s.fga.GetRole(ctx, uid, tenantID)
	if err != nil {
		return "", apperrors.Wrap(err, 500, "authz_check_failed", "failed to read caller role")
	}
	if role == "" {
		return "", apperrors.New(403, "forbidden", "you do not have a role on this tenant")
	}
	return role, nil
}

// ListMembers returns the current team for a tenant: the owner
// (from tenants.owner_email) plus every accepted invitation. Used by
// the admin Settings → Team page to render the "who's on the team"
// list. Intentionally uses the invitations table as source of truth
// rather than enumerating FGA tuples — every membership in the
// current product flow goes through an invitation, and the email
// address lives on the invitation row so we don't need an auth-bff
// round-trip to resolve UIDs.
type Member struct {
	Email      string     `json:"email"`
	Role       string     `json:"role"`
	Kind       string     `json:"kind"` // "owner" | "invited"
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
}

func (s *Service) ListMembers(ctx context.Context, tenantID string) ([]Member, error) {
	t, err := s.tenantRepo.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	accepted, err := s.repo.ListAcceptedByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]Member, 0, 1+len(accepted))
	out = append(out, Member{
		Email: t.OwnerEmail,
		Role:  "owner",
		Kind:  "owner",
	})
	for _, inv := range accepted {
		// Owner shouldn't normally accept an invitation to their own
		// tenant, but if an edge-case produced one we de-dupe by
		// email so we don't list them twice.
		if strings.EqualFold(inv.Email, t.OwnerEmail) {
			continue
		}
		out = append(out, Member{
			Email:      inv.Email,
			Role:       inv.Role,
			Kind:       "invited",
			AcceptedAt: inv.AcceptedAt,
		})
	}
	return out, nil
}

// ListPending returns the tenant's pending invitations for the
// /settings/team table. Gated on can_view_settings by the handler.
func (s *Service) ListPending(ctx context.Context, tenantID string) ([]Invitation, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, apperrors.BadRequest("invalid_tenant_id", "tenant id is required")
	}
	return s.repo.ListPendingByTenant(ctx, tenantID)
}

// Revoke marks a pending invitation as revoked. Gated on
// can_invite_members by the handler (can't invite = can't revoke).
func (s *Service) Revoke(ctx context.Context, tenantID, invitationID, uid string) error {
	tenantID = strings.TrimSpace(tenantID)
	invitationID = strings.TrimSpace(invitationID)
	uid = strings.TrimSpace(uid)
	if tenantID == "" || invitationID == "" || uid == "" {
		return apperrors.BadRequest("invalid_input", "tenant_id, invitation_id and uid are required")
	}

	if s.fga != nil {
		allowed, err := s.fga.Check(ctx, uid, "can_invite_members", tenantID)
		if err != nil {
			return apperrors.Internal("authz_check_failed", "authorization check failed")
		}
		if !allowed {
			return apperrors.New(403, "forbidden", "you do not have permission to revoke invitations")
		}
	}

	inv, err := s.repo.GetByID(ctx, invitationID)
	if err != nil {
		return err
	}
	if inv.TenantID != tenantID {
		return apperrors.NotFound("invitation_not_found", "no invitation with that id")
	}
	if err := s.repo.MarkRevoked(ctx, invitationID); err != nil {
		return err
	}
	storeIDForEvent := ""
	if inv.StoreID != nil {
		storeIDForEvent = *inv.StoreID
	}
	s.audit.EmitAsync(audit.Event{
		TenantID:     tenantID,
		StoreID:      storeIDForEvent,
		Action:       "staff.invitation_revoked",
		ResourceType: "invitation",
		ResourceID:   invitationID,
		Severity:     "warning",
		ActorType:    "user",
		ActorUserID:  uid,
		Metadata: map[string]any{
			"invited_email": inv.Email,
			"role":          inv.Role,
		},
	})
	return nil
}

// newToken generates a fresh invitation token and returns
// (plaintext, sha256_hex).
func newToken() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	plain := hex.EncodeToString(buf)
	return plain, hashToken(plain), nil
}

// profileNames returns the givenName/familyName to create a Zitadel
// profile with. Zitadel rejects an empty value for either, so when the
// accept form did not collect a name we derive one from the email local
// part rather than failing the accept over a cosmetic field: the user
// can correct it in their profile, but they cannot un-fail an accept.
//
// "jane.doe@x.com" -> ("Jane", "Doe"); "jane@x.com" -> ("Jane", "Jane").
func profileNames(first, last, email string) (string, string) {
	first = strings.TrimSpace(first)
	last = strings.TrimSpace(last)
	if first != "" && last != "" {
		return first, last
	}
	local := email
	if at := strings.Index(local, "@"); at > 0 {
		local = local[:at]
	}
	parts := strings.FieldsFunc(local, func(r rune) bool {
		return r == '.' || r == '_' || r == '-' || r == '+'
	})
	derivedFirst, derivedLast := local, local
	if len(parts) > 0 {
		derivedFirst = titleCase(parts[0])
		derivedLast = derivedFirst
		if len(parts) > 1 {
			derivedLast = titleCase(strings.Join(parts[1:], " "))
		}
	}
	if first == "" {
		first = derivedFirst
	}
	if last == "" {
		last = derivedLast
	}
	return first, last
}

// titleCase upper-cases the first rune only. strings.Title is
// deprecated and golang.org/x/text/cases would be a new dependency for
// a display-name fallback.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func hashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// inviteEmailBody / inviteEmailText have been superseded by
// notification.RenderInvitation, which renders the Paper · Ink · Moss
// invitation HTML + text templates from embedded files.
