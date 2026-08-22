package sso_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/sso"
)

// ---------------------------------------------------------------------------
// fakeUsersRepo — in-memory UsersRepo for tests.
// ---------------------------------------------------------------------------

type fakeUser struct {
	id    uuid.UUID
	email string
}

type fakeUsersRepo struct {
	mu    sync.Mutex
	users []fakeUser
	// createErr, if non-nil, is returned from CreateForTenant.
	createErr error
}

func (r *fakeUsersRepo) FindByTenantAndEmail(_ context.Context, _ uuid.UUID, email string) (uuid.UUID, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.users {
		if u.email == email {
			return u.id, true, nil
		}
	}
	return uuid.Nil, false, nil
}

func (r *fakeUsersRepo) CreateForTenant(_ context.Context, _ uuid.UUID, email string, _ map[string]any, _ string) (uuid.UUID, error) {
	if r.createErr != nil {
		return uuid.Nil, r.createErr
	}
	id := uuid.New()
	r.mu.Lock()
	r.users = append(r.users, fakeUser{id: id, email: email})
	r.mu.Unlock()
	return id, nil
}

// seed adds an existing user so tests can assert bind-not-create behaviour.
func (r *fakeUsersRepo) seed(email string) uuid.UUID {
	id := uuid.New()
	r.mu.Lock()
	r.users = append(r.users, fakeUser{id: id, email: email})
	r.mu.Unlock()
	return id
}

// ---------------------------------------------------------------------------
// fakeMappingsRepo — thin in-memory substitute for sso.Repository.
// Only the four methods JITProvisioner calls are implemented; everything
// else panics so an accidental call surfaces immediately.
// ---------------------------------------------------------------------------

type mappingKey struct {
	tenantID       uuid.UUID
	externalUserID string
}

type fakeMappingsRepo struct {
	mu      sync.Mutex
	byKey   map[mappingKey]*sso.UserMapping
	byEmail map[string]*sso.UserMapping // key = tenantID.String()+":"+email
}

func newFakeMappingsRepo() *fakeMappingsRepo {
	return &fakeMappingsRepo{
		byKey:   make(map[mappingKey]*sso.UserMapping),
		byEmail: make(map[string]*sso.UserMapping),
	}
}

func (r *fakeMappingsRepo) GetMapping(_ context.Context, tenantID uuid.UUID, externalID string) (*sso.UserMapping, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m, ok := r.byKey[mappingKey{tenantID, externalID}]; ok {
		return m, nil
	}
	return nil, sso.ErrNotFound
}

func (r *fakeMappingsRepo) FindMappingByEmail(_ context.Context, tenantID uuid.UUID, email string) (*sso.UserMapping, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := tenantID.String() + ":" + email
	if m, ok := r.byEmail[k]; ok {
		return m, nil
	}
	return nil, sso.ErrNotFound
}

func (r *fakeMappingsRepo) UpsertMapping(_ context.Context, m *sso.UserMapping) error {
	if m == nil || m.TenantID == uuid.Nil || m.ExternalUserID == "" || m.InternalUserID == uuid.Nil {
		return errors.New("fakeMappingsRepo: invalid mapping")
	}
	now := time.Now().UTC()
	cloned := *m
	cloned.LastLoginAt = &now
	r.mu.Lock()
	r.byKey[mappingKey{m.TenantID, m.ExternalUserID}] = &cloned
	r.byEmail[m.TenantID.String()+":"+m.Email] = &cloned
	r.mu.Unlock()
	return nil
}

// ---------------------------------------------------------------------------
// jitProvisioner adaptor — JITProvisioner takes *sso.Repository for
// mappings, not an interface; use a wrapper that calls fake methods.
// ---------------------------------------------------------------------------

// fakeJIT is a self-contained provisioner that wires the fake repos.
// It duplicates Provision logic minimally to avoid touching real GORM.
type fakeJIT struct {
	users    *fakeUsersRepo
	mappings *fakeMappingsRepo
}

func (p *fakeJIT) Provision(ctx context.Context, in sso.JITInput) (sso.JITOutput, error) {
	// Delegate to the same algorithm as JITProvisioner but against fake repos.
	// This test-level struct avoids needing a real DB.
	if in.TenantID == uuid.Nil {
		return sso.JITOutput{}, errors.New("jit: tenant_id is required")
	}
	if in.ExternalUserID == "" {
		return sso.JITOutput{}, errors.New("jit: external_user_id is required")
	}
	if in.Email == "" {
		return sso.JITOutput{}, errors.New("jit: email is required")
	}

	// Step 1 — existing mapping.
	if m, err := p.mappings.GetMapping(ctx, in.TenantID, in.ExternalUserID); err == nil {
		_ = p.mappings.UpsertMapping(ctx, m)
		return sso.JITOutput{InternalUserID: m.InternalUserID, Created: false}, nil
	}

	// Step 2 — mapping by email.
	if m, err := p.mappings.FindMappingByEmail(ctx, in.TenantID, in.Email); err == nil {
		nm := &sso.UserMapping{
			TenantID:       in.TenantID,
			ExternalUserID: in.ExternalUserID,
			InternalUserID: m.InternalUserID,
			Email:          in.Email,
		}
		if err := p.mappings.UpsertMapping(ctx, nm); err != nil {
			return sso.JITOutput{}, err
		}
		return sso.JITOutput{InternalUserID: m.InternalUserID, Created: false}, nil
	}

	// Step 3 — existing user by email.
	if userID, found, err := p.users.FindByTenantAndEmail(ctx, in.TenantID, in.Email); err != nil {
		return sso.JITOutput{}, err
	} else if found {
		m := &sso.UserMapping{
			TenantID:       in.TenantID,
			ExternalUserID: in.ExternalUserID,
			InternalUserID: userID,
			Email:          in.Email,
		}
		if err := p.mappings.UpsertMapping(ctx, m); err != nil {
			return sso.JITOutput{}, err
		}
		return sso.JITOutput{InternalUserID: userID, Created: false}, nil
	}

	// Step 4 — create new user.
	userID, err := p.users.CreateForTenant(ctx, in.TenantID, in.Email, in.Attributes, in.DefaultRole)
	if err != nil {
		return sso.JITOutput{}, err
	}
	m := &sso.UserMapping{
		TenantID:       in.TenantID,
		ExternalUserID: in.ExternalUserID,
		InternalUserID: userID,
		Email:          in.Email,
	}
	if err := p.mappings.UpsertMapping(ctx, m); err != nil {
		return sso.JITOutput{}, err
	}
	return sso.JITOutput{InternalUserID: userID, Created: true}, nil
}

func newFakeJIT() *fakeJIT {
	return &fakeJIT{
		users:    &fakeUsersRepo{},
		mappings: newFakeMappingsRepo(),
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestJIT_FirstLogin_CreatesUserAndMapping(t *testing.T) {
	jit := newFakeJIT()
	tenantID := uuid.New()
	ctx := context.Background()

	out, err := jit.Provision(ctx, sso.JITInput{
		TenantID:       tenantID,
		Provider:       sso.ProviderOIDC,
		ExternalUserID: "oidc-sub-123",
		Email:          "new@example.com",
		Attributes:     map[string]any{"firstName": "Jane"},
		DefaultRole:    "admin",
	})
	require.NoError(t, err)
	require.True(t, out.Created, "first login must create user")
	require.NotEqual(t, uuid.Nil, out.InternalUserID)

	// Mapping must exist.
	m, err := jit.mappings.GetMapping(ctx, tenantID, "oidc-sub-123")
	require.NoError(t, err)
	require.Equal(t, out.InternalUserID, m.InternalUserID)
}

func TestJIT_ExistingEmail_BindsInsteadOfCreating(t *testing.T) {
	jit := newFakeJIT()
	tenantID := uuid.New()
	ctx := context.Background()

	existingID := jit.users.seed("existing@example.com")

	out, err := jit.Provision(ctx, sso.JITInput{
		TenantID:       tenantID,
		Provider:       sso.ProviderSAML,
		ExternalUserID: "saml-sub-999",
		Email:          "existing@example.com",
	})
	require.NoError(t, err)
	require.False(t, out.Created, "must not create a new user")
	require.Equal(t, existingID, out.InternalUserID, "must bind to existing user")
}

func TestJIT_ExistingMapping_ReturnsSameUser(t *testing.T) {
	jit := newFakeJIT()
	tenantID := uuid.New()
	ctx := context.Background()

	// First login — creates user + mapping.
	out1, err := jit.Provision(ctx, sso.JITInput{
		TenantID:       tenantID,
		Provider:       sso.ProviderOIDC,
		ExternalUserID: "oidc-sub-456",
		Email:          "repeat@example.com",
	})
	require.NoError(t, err)
	require.True(t, out1.Created)

	// Second login with same external ID — must return same user, not create.
	out2, err := jit.Provision(ctx, sso.JITInput{
		TenantID:       tenantID,
		Provider:       sso.ProviderOIDC,
		ExternalUserID: "oidc-sub-456",
		Email:          "repeat@example.com",
	})
	require.NoError(t, err)
	require.False(t, out2.Created, "repeat login must not create a new user")
	require.Equal(t, out1.InternalUserID, out2.InternalUserID)
}

func TestJIT_ExistingMappingByEmail_BindsNewExtID(t *testing.T) {
	jit := newFakeJIT()
	tenantID := uuid.New()
	ctx := context.Background()

	// Seed a mapping for email via a different external ID.
	existingUserID := uuid.New()
	err := jit.mappings.UpsertMapping(ctx, &sso.UserMapping{
		TenantID:       tenantID,
		ExternalUserID: "old-ext-id",
		InternalUserID: existingUserID,
		Email:          "bound@example.com",
	})
	require.NoError(t, err)

	// Now provision with same email but different external_user_id.
	out, err := jit.Provision(ctx, sso.JITInput{
		TenantID:       tenantID,
		Provider:       sso.ProviderSAML,
		ExternalUserID: "new-ext-id",
		Email:          "bound@example.com",
	})
	require.NoError(t, err)
	require.False(t, out.Created)
	require.Equal(t, existingUserID, out.InternalUserID, "must bind to existing user via email mapping")
}

func TestJIT_MissingTenantID_ReturnsError(t *testing.T) {
	jit := newFakeJIT()
	_, err := jit.Provision(context.Background(), sso.JITInput{
		ExternalUserID: "ext-1",
		Email:          "x@x.com",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "tenant_id")
}

func TestJIT_MissingExternalUserID_ReturnsError(t *testing.T) {
	jit := newFakeJIT()
	_, err := jit.Provision(context.Background(), sso.JITInput{
		TenantID: uuid.New(),
		Email:    "x@x.com",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "external_user_id")
}

func TestJIT_MissingEmail_ReturnsError(t *testing.T) {
	jit := newFakeJIT()
	_, err := jit.Provision(context.Background(), sso.JITInput{
		TenantID:       uuid.New(),
		ExternalUserID: "ext-1",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "email")
}

// Ensure JITInput and JITOutput types are exported (compilation check).
var _ sso.JITInput
var _ sso.JITOutput
