package onboarding

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/marketplaceapi"
	"github.com/mark8ly/platform-api/internal/notification"
	"github.com/mark8ly/platform-api/internal/outbox"
	"github.com/mark8ly/platform-api/internal/store"
	"github.com/mark8ly/platform-api/internal/tenant"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// VendorEnsurer lets the Service call marketplace-api without a hard
// dependency on the concrete HTTP client. The marketplaceapi.VendorClient
// satisfies this interface directly.
//
// EnsureSelfStore mirrors the authoritative platform-api store row into
// marketplace-api's local stores projection; without it, downstream slug
// lookups (admin dashboard, storefront, custom-domain join) 404 because
// marketplace_api.stores is empty for the new tenant.
type VendorEnsurer interface {
	EnsureSelfVendor(ctx context.Context, tenantID, name, slug string) (*marketplaceapi.Vendor, error)
	EnsureSelfStore(ctx context.Context, s marketplaceapi.Store) (*marketplaceapi.Store, error)
}

// Service is the business logic for the onboarding flow.
//
// The Complete() method is the bug-fix landing point for auth-bug #2 (FGA
// tuple race) and #3 (no retry on FGA failure) from
// docs/planning/auth-bugs.md. The fix has three parts:
//
//  1. The DB transaction creates BOTH the tenant row AND the outbox row
//     describing the FGA tuple writes. Either both commit or neither does.
//  2. The outbox drainer (separate goroutine) ships the tuple to OpenFGA
//     with retries. If FGA is down, the row stays pending and is retried.
//  3. (Out of this package's scope) auth-bff's auto-login does an FGA
//     Check with retry-on-not-found, so the user can't reach admin until
//     the tuple is actually visible.
//
// Together these close the race window from both ends.
// OwnerProvisioner makes the onboarding merchant a sign-in-capable
// identity in the identity provider and returns their provider user id.
//
// Non-nil ONLY on the Zitadel path (see cmd/server/provider_wiring.go's
// newOwnerProvisioner). Nil selects the GIP path, whose behaviour is
// unchanged: under GIP the set-password form creates the account
// client-side before it ever calls platform-api, so there is nothing to
// provision server-side and the caller-supplied owner_user_id is already
// a real identity.
//
// Satisfied by *zitadeladmin.StaffProvisioner — the SAME type
// internal/invitation uses, and deliberately so. The merchant needs
// exactly what an invited teammate needs: a Zitadel account and a grant
// on the mark8ly-admin project, whose projectRoleCheck: true otherwise
// makes OIDC finalize return 403 OIDC-foSyH49RvL. There is only one role
// on that project (mark8ly.staff) and it gates PROJECT ACCESS, not
// authority — the owner/staff distinction lives entirely in FGA, which
// is why the owner reuses the staff role key here and still gets an FGA
// `owner` tuple below.
type OwnerProvisioner interface {
	ProvisionStaff(ctx context.Context, email, firstName, lastName, password string) (string, error)
}

type Service struct {
	db                    *gorm.DB
	repo                  Repository
	tenantRepo            tenant.Repository
	storeRepo             store.Repository
	sender                notification.Sender
	loader                *notification.Loader
	emailFrom             string
	supportSite           string
	adminURLTemplate      string
	storefrontURLTemplate string
	vendorClient          VendorEnsurer
	provisioner           OwnerProvisioner
}

// Config holds Service dependencies.
type Config struct {
	DB         *gorm.DB
	Repo       Repository
	TenantRepo tenant.Repository
	StoreRepo  store.Repository
	Sender     notification.Sender
	// Loader is the DB-backed template loader (embedded fallback).
	Loader    *notification.Loader
	EmailFrom string
	// AdminURLTemplate is a template like "https://%s-admin.mark8ly.com"
	// for building the welcome email's admin link. Supports both the
	// per-slug shape (contains %s) and flat-host shape (no %s → used
	// as-is, for dev on localhost).
	AdminURLTemplate      string
	StorefrontURLTemplate string
	SupportEmail          string
	// VendorClient is called best-effort after onboarding commits to
	// ensure marketplace-api has a self-vendor for the new tenant.
	// A nil VendorClient disables the call (useful for tests that don't
	// need the vendor path).
	VendorClient VendorEnsurer
	// Provisioner creates the owner's identity-provider account and admin
	// project grant during Complete. Nil on the GIP path — see
	// OwnerProvisioner's doc.
	Provisioner OwnerProvisioner
}

// NewService constructs a Service.
func NewService(cfg Config) *Service {
	return &Service{
		db:                    cfg.DB,
		repo:                  cfg.Repo,
		tenantRepo:            cfg.TenantRepo,
		storeRepo:             cfg.StoreRepo,
		sender:                cfg.Sender,
		loader:                cfg.Loader,
		emailFrom:             cfg.EmailFrom,
		supportSite:           cfg.SupportEmail,
		adminURLTemplate:      cfg.AdminURLTemplate,
		storefrontURLTemplate: cfg.StorefrontURLTemplate,
		vendorClient:          cfg.VendorClient,
		provisioner:           cfg.Provisioner,
	}
}

// CreateRequest is the input to Create.
type CreateRequest struct {
	Email string `json:"email"`
}

// Create starts a new onboarding session for the given email. Idempotent
// in spirit (same email can start multiple sessions if previous ones were
// abandoned), but each call always returns a fresh session ID.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Session, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || !strings.Contains(email, "@") {
		return nil, apperrors.BadRequest("invalid_email", "valid email is required")
	}

	// An admin email is globally unique across tenants. Reject here, at
	// step 1 of the wizard, rather than letting the merchant fill in the
	// whole form and hit the failure at the final insert (or worse, at
	// GIP's EMAIL_EXISTS on the set-password screen, which reads as an
	// auth error rather than "pick a different email").
	if err := s.ensureOwnerEmailAvailable(ctx, email); err != nil {
		return nil, err
	}

	sess := &Session{
		Email:  email,
		Draft:  json.RawMessage(`{}`),
		Status: StatusInProgress,
	}
	if err := s.repo.Create(ctx, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// ensureOwnerEmailAvailable rejects an email that already owns a tenant.
//
// This is the authoritative cross-tenant admin-email guard. It runs on
// BOTH Create (fail fast at step 1) and Complete (non-bypassable — a
// client calling POST /onboarding/sessions/:id/complete directly still
// hits it). The frontend check in apps/onboarding/app/onboarding/
// actions.ts is a UX affordance only; this is the enforcement point.
//
// The DB-level backstop is the tenants_owner_email_unique index
// (migration 0014), which closes the TOCTOU window between this check
// and the insert — two concurrent onboardings for the same email make
// one of them fail at CreateInTx with a 23505 rather than both landing.
func (s *Service) ensureOwnerEmailAvailable(ctx context.Context, email string) error {
	if s.tenantRepo == nil {
		return nil
	}
	exists, err := s.tenantRepo.OwnerEmailExists(ctx, email)
	if err != nil {
		return err
	}
	if exists {
		return apperrors.Conflict(
			"owner_email_already_in_use",
			"This email is already an admin of another store. Please use a different email address — an admin email must be unique across stores.",
		)
	}
	return nil
}

// Get returns a session by ID.
func (s *Service) Get(ctx context.Context, id string) (*Session, error) {
	return s.repo.GetByID(ctx, id)
}

// GetFunnel returns the onboarding funnel counters for the given window
// (#283). AsOf is deliberately not settable from outside this package —
// see FunnelFilter's doc comment.
func (s *Service) GetFunnel(ctx context.Context, f FunnelFilter) (*FunnelStats, error) {
	return s.repo.GetFunnel(ctx, f)
}

// ListSessions returns a page of onboarding sessions for the given
// window/filter, plus the unpaginated total (#283).
func (s *Service) ListSessions(ctx context.Context, f FunnelFilter) ([]SessionRow, int64, error) {
	return s.repo.ListSessions(ctx, f)
}

// SaveDraft replaces the session's draft JSON. The draft is opaque to the
// backend — the wizard's frontend owns the schema. We only validate that
// it parses as JSON.
func (s *Service) SaveDraft(ctx context.Context, id string, draft json.RawMessage) error {
	if !json.Valid(draft) {
		return apperrors.BadRequest("invalid_draft", "draft must be valid JSON")
	}
	return s.repo.UpdateDraft(ctx, id, draft)
}

// MarkVerified is called by the verification service after a successful
// OTP verification. Idempotent.
func (s *Service) MarkVerified(ctx context.Context, sessionID string) error {
	return s.repo.MarkEmailVerified(ctx, sessionID)
}

// CompleteRequest is the input to Complete. It contains the merchant's
// final answers — everything needed to mint a tenant.
type CompleteRequest struct {
	SessionID    string `json:"session_id"`
	BusinessName string `json:"business_name"`
	Slug         string `json:"slug"`
	// OwnerUserID is the GIP UID of the account the set-password form
	// already created. Required on the GIP path ONLY. On the Zitadel path
	// the merchant has no provider account at this point — creating one is
	// what Complete does — so there is no id for the caller to send and
	// the field is ignored.
	OwnerUserID string `json:"owner_user_id"`
	OwnerEmail  string `json:"owner_email"`
	// Password is the password to create the owner's Zitadel account
	// with. Zitadel path only, and only consulted when no account exists
	// for the address yet. Ignored entirely on the GIP path.
	Password string `json:"password"`
	// FirstName / LastName populate the Zitadel profile. Optional —
	// derived from the email local part when absent (Zitadel rejects an
	// empty givenName/familyName).
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	CountryCode  string `json:"country_code"`
	CurrencyCode string `json:"currency_code"`
	Timezone     string `json:"timezone"`
}

// CompleteResult is what Complete returns on success.
type CompleteResult struct {
	TenantID string `json:"tenant_id"`
	Slug     string `json:"slug"`
}

// fgaWritePayload is the outbox event payload for an FGA tuple write.
// Stored in outbox_events.payload as JSONB; consumed by the registered
// handler in cmd/server.
//
// Phase Q added StoreID: the drainer also writes the
// `store:<id> parent tenant:<id>` tuple so FGA's store-type
// `from parent` inheritance works for the default store created
// during onboarding.
type fgaWritePayload struct {
	// UserID is the single subject the GIP path writes, and is what every
	// outbox row enqueued before #685 carries. The handler still reads
	// it, so rows left pending across the rollout drain unchanged.
	UserID string `json:"user_id"`
	// UserIDs is the Zitadel path's subject list: the owner's lowercased
	// EMAIL and their Zitadel user id, both written as owner tuples.
	//
	// Two keys because three readers disagree on what a member is keyed
	// by, exactly as #679/#680 found for invitation accept:
	//
	//   - admin sign-in (apps/admin/app/login/actions.ts,
	//     resolveWorkspaceTenant) looks membership up by EMAIL on the
	//     Zitadel path — tenant resolution runs BEFORE authentication, so
	//     at that moment the typed address is the only identifier in
	//     existence and there is no `sub` to key on;
	//   - the bearer-token API path resolves by the Zitadel uid;
	//   - this code used to write only the GIP uid, which after the
	//     Zitadel cutover matches neither — the bug #685 reports.
	//
	// Writing one key breaks the other reader: email-only 403s every API
	// call, uid-only puts the merchant back on "We couldn't find a store
	// for this account. Did you finish onboarding?" — the message they
	// are shown seconds after finishing onboarding.
	UserIDs  []string `json:"user_ids,omitempty"`
	TenantID string   `json:"tenant_id"`
	StoreID  string   `json:"store_id,omitempty"`
}

// FGAOutboxKind is the dispatch key for FGA membership writes.
const FGAOutboxKind = "fga.write_membership"

// Complete is the bug-fix transaction.
//
// In one DB transaction:
//
//  1. Verify the session is in a completable state (verified email)
//  2. Create the tenant row
//  3. Mark the session completed and link tenant_id
//  4. Enqueue an outbox event for the FGA membership write
//
// All four happen-or-nothing. The outbox drainer picks up the event and
// writes the OpenFGA tuple. If the drainer is delayed or FGA is down,
// the membership tuple is eventually written; the user just sees auth-bff
// retry on auto-login until the tuple appears.
//
// After commit (NOT in the tx) we send the welcome email best-effort.
// Failure to send is logged but does not fail completion.
func (s *Service) Complete(ctx context.Context, req CompleteRequest) (*CompleteResult, error) {
	// owner_user_id is required on the GIP path only. With a provisioner
	// wired (Zitadel) the merchant has no provider account yet, so there
	// is no uid to send — this call is what creates one. The error code
	// and message are unchanged so the GIP path's response stays
	// byte-identical.
	if err := validateCompleteRequest(req, s.provisioner == nil); err != nil {
		return nil, err
	}

	// Snapshot the session before the tx — so we can email the user even
	// if we don't need the row data inside the tx.
	sess, err := s.repo.GetByID(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}
	if sess.Status == StatusCompleted {
		return nil, apperrors.Conflict("already_completed", "session is already completed")
	}
	if sess.EmailVerifiedAt == nil {
		return nil, apperrors.BadRequest("email_not_verified", "email must be verified before completion")
	}

	// Re-check at completion. Create already rejected a duplicate, but a
	// tenant may have claimed this email while the wizard was open, and
	// a direct API call skips Create's check entirely.
	if err := s.ensureOwnerEmailAvailable(ctx, req.OwnerEmail); err != nil {
		return nil, err
	}

	// Provision the merchant in the identity provider (Zitadel path only;
	// s.provisioner is nil under GIP). This runs BEFORE the transaction,
	// and a failure aborts Complete with no tenant, no store and no
	// outbox row written.
	//
	// That ordering is the point, and it is the same one
	// invitation.Accept uses. The failure worth preventing is a
	// HALF-provisioned merchant — a tenant whose owner has no Zitadel
	// account or no project grant — which looks like a finished signup
	// right up until the first sign-in fails, and which the merchant
	// cannot repair themselves. The reverse leak, a Zitadel account
	// created for a completion that then fails at the DB, IS possible and
	// is deliberately the cheaper one: ProvisionStaff resolves an
	// existing account instead of failing, so the retry the merchant is
	// already going to make lands on the same user, and an account with a
	// project grant but no tenant cannot sign in to anything.
	ownerUserID := strings.TrimSpace(req.OwnerUserID)
	ownerEmail := strings.ToLower(strings.TrimSpace(req.OwnerEmail))
	// subjects are the FGA tuple keys: one under GIP (the uid the form
	// already created), two under Zitadel — see fgaWritePayload.UserIDs.
	subjects := []string{ownerUserID}
	if s.provisioner != nil {
		first, last := profileNames(req.FirstName, req.LastName, ownerEmail)
		zitadelUID, provErr := s.provisioner.ProvisionStaff(ctx, ownerEmail, first, last, req.Password)
		if provErr != nil {
			return nil, provisioningError(provErr, req.SessionID, ownerEmail, req.Password)
		}
		ownerUserID = zitadelUID
		// Email first, lowercased: the login path folds to lower
		// server-side and every email-keyed tuple in the store is
		// lowercase, so `Founder@Example.com` written verbatim would miss
		// its own membership.
		subjects = []string{ownerEmail, zitadelUID}
	}

	// Phase Q: onboarding creates BOTH a tenant (the company) and a
	// default store (the first storefront) in the same transaction.
	// The merchant's "business name" becomes both the tenant.name
	// (company) and the store.name ("Main Store" label is reserved
	// for backfilled legacy tenants only). The store row carries the
	// slug + currency + timezone + country — the user-facing
	// storefront identity.
	t := &tenant.Tenant{
		Name: req.BusinessName,
		// The Zitadel user id on the Zitadel path, not the caller's: it is
		// the id the bearer API sees and the one every later membership
		// write keys on.
		OwnerUserID: ownerUserID,
		// Store normalized so the row matches what the case-insensitive
		// uniqueness check and the lower(owner_email) index compare on.
		OwnerEmail: ownerEmail,
		Status:     tenant.StatusActive,
	}
	st := &store.Store{
		Slug:            req.Slug,
		Name:            req.BusinessName,
		CountryCode:     req.CountryCode,
		CurrencyCode:    req.CurrencyCode,
		Timezone:        req.Timezone,
		StorefrontTheme: json.RawMessage(`{}`),
		Status:          store.StatusActive,
	}

	// THE BUG-FIX TRANSACTION (Phase D) extended with the Phase Q
	// store creation. Either every row commits together or nothing
	// does — if the store insert fails on a slug collision, the
	// tenant row is rolled back along with it.
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.tenantRepo.CreateInTx(ctx, tx, t); err != nil {
			return err
		}
		st.TenantID = t.ID
		if err := s.storeRepo.CreateInTx(ctx, tx, st); err != nil {
			return err
		}
		if err := s.repo.CompleteInTx(ctx, tx, req.SessionID, t.ID); err != nil {
			return err
		}
		// Phase D outbox event — owner tuple on the tenant. Store-
		// level tuples come with Phase R (store-level invites).
		if err := outbox.Enqueue(tx, FGAOutboxKind, fgaWritePayload{
			UserID:   ownerUserID,
			UserIDs:  subjects,
			TenantID: t.ID,
			StoreID:  st.ID,
		}); err != nil {
			return err
		}
		// Stamp the owner's tenant_id GIP custom claim. marketplace-api's
		// mobile auth resolves the caller's tenant from this claim alone,
		// so without it the owner authenticates but every mobile API call
		// is refused — which the app renders as a login bounce loop. Rides
		// the same transaction/outbox as the FGA tuple so it is retried
		// until it lands rather than silently locking the owner out.
		//
		// Skipped on the Zitadel path: ownerUserID is a Zitadel user id
		// there and EnsureTenantClaim writes a GIP custom claim keyed by
		// GIP uid. The call would resolve nothing, fail, and be retried by
		// the drainer forever. The GIP path is untouched. (Mobile admin is
		// on the same GIP bearer path and is a separate cutover — see
		// cmd/server/provider_wiring.go's requireGIPForTenantClaim.)
		if s.provisioner == nil {
			if err := outbox.Enqueue(tx, GIPClaimOutboxKind, gipClaimPayload{
				UserID:   ownerUserID,
				TenantID: t.ID,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Phase 1 of the tenant/vendor/store refactor: create the tenant's
	// self-vendor in marketplace-api. Best-effort — a failure is logged
	// but does NOT fail onboarding. The platform-api backfill CLI
	// (cmd/backfill-vendors) covers any misses.
	if s.vendorClient != nil {
		if _, vErr := s.vendorClient.EnsureSelfVendor(ctx, t.ID, t.Name, st.Slug); vErr != nil {
			log.Printf("onboarding.Complete: ensure self-vendor for tenant %s: %v", t.ID, vErr)
		}
		// Mirror the authoritative store row into marketplace_api.stores
		// so admin/storefront slug lookups resolve. Same best-effort
		// policy as EnsureSelfVendor — a failure logs but doesn't fail
		// onboarding (merchant can re-trigger via store-settings update).
		if _, sErr := s.vendorClient.EnsureSelfStore(ctx, marketplaceapi.Store{
			ID:           st.ID,
			TenantID:     t.ID,
			Slug:         st.Slug,
			Name:         st.Name,
			CountryCode:  st.CountryCode,
			CurrencyCode: st.CurrencyCode,
			Timezone:     st.Timezone,
			Status:       string(store.StatusActive),
		}); sErr != nil {
			log.Printf("onboarding.Complete: ensure self-store for tenant %s store %s: %v", t.ID, st.ID, sErr)
		}
	}

	if s.sender != nil {
		_ = s.sendWelcome(ctx, t, st, req)
	}

	return &CompleteResult{TenantID: t.ID, Slug: st.Slug}, nil
}

func (s *Service) sendWelcome(ctx context.Context, t *tenant.Tenant, st *store.Store, req CompleteRequest) error {
	vars := notification.WelcomeVars{
		BusinessName:  t.Name,
		AdminURL:      formatURLTemplate(s.adminURLTemplate, st.Slug),
		StorefrontURL: formatURLTemplate(s.storefrontURLTemplate, st.Slug),
		SupportEmail:  s.supportSite,
	}
	// DB-loaded template with embedded fallback. Tenant ID is freshly
	// committed so we forward it for engagement attribution.
	var (
		msg notification.Email
		err error
	)
	if s.loader != nil {
		msg, err = s.loader.Render(ctx, "welcome", t.OwnerEmail, s.emailFrom, t.ID, vars)
	} else {
		msg, err = notification.RenderWelcome(t.OwnerEmail, s.emailFrom, vars)
		if err == nil {
			msg.TenantID = t.ID
		}
	}
	if err != nil {
		return err
	}
	return s.sender.Send(ctx, msg)
}

// formatURLTemplate substitutes the tenant slug into a URL template.
// Supports both Go's printf "%s" verb and the {slug} placeholder used
// by the Helm charts. Falls back to the template as-is when neither
// is present (flat-host shape for dev on localhost).
func formatURLTemplate(template, slug string) string {
	if strings.Contains(template, "{slug}") {
		return strings.ReplaceAll(template, "{slug}", slug)
	}
	if strings.Contains(template, "%s") {
		return fmt.Sprintf(template, slug)
	}
	return template
}

// validateCompleteRequest enforces presence + shape of every required
// field. requireOwnerUserID is false on the Zitadel path, where the
// provider account does not exist yet — see CompleteRequest.OwnerUserID.
func validateCompleteRequest(req CompleteRequest, requireOwnerUserID bool) error {
	if req.SessionID == "" {
		return apperrors.BadRequest("invalid_session", "session_id is required")
	}
	if strings.TrimSpace(req.BusinessName) == "" {
		return apperrors.BadRequest("invalid_business_name", "business_name is required")
	}
	if req.Slug == "" {
		return apperrors.BadRequest("invalid_slug", "slug is required")
	}
	if requireOwnerUserID && req.OwnerUserID == "" {
		return apperrors.BadRequest("invalid_owner", "owner_user_id is required")
	}
	if req.OwnerEmail == "" || !strings.Contains(req.OwnerEmail, "@") {
		return apperrors.BadRequest("invalid_owner_email", "owner_email is required and must be valid")
	}
	if len(req.CountryCode) != 2 {
		return apperrors.BadRequest("invalid_country_code", "country_code must be a 2-letter ISO code")
	}
	if len(req.CurrencyCode) != 3 {
		return apperrors.BadRequest("invalid_currency_code", "currency_code must be a 3-letter ISO 4217 code")
	}
	if req.Timezone == "" {
		return apperrors.BadRequest("invalid_timezone", "timezone is required")
	}
	return nil
}

// profileNames resolves the givenName/familyName Zitadel requires,
// falling back to the email local part when the wizard sent nothing.
// Zitadel rejects an empty name outright, so "no name" is not an option
// and a derived one beats a failed signup.
//
// Deliberately a copy of internal/invitation's identical helper rather
// than a shared import: the two packages are independent of each other
// by design, and a naming heuristic is not worth a new shared package
// that couples them.
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

// titleCase upper-cases the first rune only.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}
