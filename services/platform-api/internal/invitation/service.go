package invitation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

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
	repo       Repository
	tenantRepo tenant.Repository
	storeRepo  store.Repository
	fga        authz.Client
	sender     notification.Sender
	emailFrom  string
	acceptURL  func(slug, token string) string
	expiry     time.Duration
	recorder   TokenRecorder
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
	EmailFrom string
	// AcceptURL builds the full invitation accept URL from the
	// default-store slug and a plaintext token.
	AcceptURL func(slug, token string) string
	// Expiry defaults to 72h when zero.
	Expiry   time.Duration
	Recorder TokenRecorder
}

// NewService constructs a Service.
func NewService(cfg Config) *Service {
	if cfg.Expiry == 0 {
		cfg.Expiry = 72 * time.Hour
	}
	return &Service{
		repo:       cfg.Repo,
		tenantRepo: cfg.TenantRepo,
		storeRepo:  cfg.StoreRepo,
		fga:        cfg.FGA,
		sender:     cfg.Sender,
		emailFrom:  cfg.EmailFrom,
		acceptURL:  cfg.AcceptURL,
		expiry:     cfg.Expiry,
		recorder:   cfg.Recorder,
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
		msg, err := notification.RenderInvitation(email, s.emailFrom, notification.InvitationVars{
			TenantName:   t.Name,
			Role:         role,
			AcceptURL:    url,
			ExpiresIn:    "72 hours",
			SupportEmail: s.emailFrom,
		})
		if err == nil {
			_ = s.sender.Send(ctx, msg)
		}
	}

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
	UID string
	// VerifiedEmail is the GIP token's verified email claim.
	// Must match the invitation's email (case-insensitive).
	VerifiedEmail string
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
	if token == "" || uid == "" || verifiedEmail == "" {
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
		if inv.StoreID != nil && *inv.StoreID != "" {
			if err := s.fga.WriteRoleObject(ctx, uid, inv.Role, "store", *inv.StoreID); err != nil {
				return nil, apperrors.Wrap(err, 500, "authz_write_failed", "failed to grant store role")
			}
			// Back-fill a tenant viewer so session mint works.
			if err := s.fga.WriteRole(ctx, uid, authz.RoleViewer, inv.TenantID); err != nil {
				return nil, apperrors.Wrap(err, 500, "authz_write_failed", "failed to grant tenant viewer")
			}
		} else {
			role := authz.Role(inv.Role)
			if err := s.fga.WriteRole(ctx, uid, role, inv.TenantID); err != nil {
				return nil, apperrors.Wrap(err, 500, "authz_write_failed", "failed to grant role")
			}
		}
	}

	if err := s.repo.MarkAccepted(ctx, inv.ID, uid); err != nil {
		return nil, err
	}
	return &AcceptResult{
		TenantID: inv.TenantID,
		Role:     inv.Role,
	}, nil
}

// UpdateMemberRoleInput is the request for the role-change endpoint.
type UpdateMemberRoleInput struct {
	TenantID   string
	TargetEmail string
	NewRole    string
	ActorUID   string
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
	}

	// Update the display label on the invitation row so ListMembers
	// reflects the new role without another FGA roundtrip.
	if err := s.repo.UpdateRoleByEmail(ctx, tenantID, targetEmail, newRole); err != nil {
		return nil, err
	}

	return &UpdateMemberRoleResult{Email: targetEmail, Role: newRole}, nil
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
	return s.repo.MarkRevoked(ctx, invitationID)
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

func hashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// inviteEmailBody / inviteEmailText have been superseded by
// notification.RenderInvitation, which renders the Paper · Ink · Moss
// invitation HTML + text templates from embedded files.
