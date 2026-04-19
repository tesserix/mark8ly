package sso

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repository is the persistence boundary for tenant_sso_configs and
// tenant_sso_user_mappings. Every method takes tenantID as the first
// parameter and pushes a WHERE tenant_id = ? into the generated SQL —
// the repo refuses to operate without a scope.
type Repository struct {
	db *gorm.DB
}

// NewRepository returns a Repository bound to db.
func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) ctx(ctx context.Context) *gorm.DB {
	if ctx == nil {
		return r.db
	}
	return r.db.WithContext(ctx)
}

// Validate checks the provider + metadata shape before any write.
// Repositories call this from Upsert; handlers can call it earlier to
// surface 4xx before disk I/O.
func Validate(cfg *Config) error {
	if cfg == nil {
		return ErrInvalidMetadata
	}
	if cfg.TenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id required", ErrInvalidMetadata)
	}
	if !cfg.Provider.Valid() {
		return ErrInvalidProvider
	}
	if cfg.Metadata == nil {
		return fmt.Errorf("%w: metadata required", ErrInvalidMetadata)
	}
	switch cfg.Provider {
	case ProviderSAML:
		for _, k := range []string{SAMLKeyIDPEntityID, SAMLKeyIDPACSURL, SAMLKeyIDPCertPEM} {
			if _, ok := nonEmpty(cfg.Metadata, k); !ok {
				return fmt.Errorf("%w: saml.%s required", ErrInvalidMetadata, k)
			}
		}
	case ProviderOIDC:
		for _, k := range []string{OIDCKeyIssuer, OIDCKeyClientID, OIDCKeyDiscoveryURL} {
			if _, ok := nonEmpty(cfg.Metadata, k); !ok {
				return fmt.Errorf("%w: oidc.%s required", ErrInvalidMetadata, k)
			}
		}
	}
	return nil
}

func nonEmpty(m map[string]interface{}, k string) (string, bool) {
	v, ok := m[k]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

// GetByTenant returns the one Config row for a tenant, or ErrNotFound.
func (r *Repository) GetByTenant(ctx context.Context, tenantID uuid.UUID) (*Config, error) {
	var c Config
	err := r.ctx(ctx).
		Where("tenant_id = ?", tenantID).
		First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Upsert inserts or updates the config for cfg.TenantID. Validate is
// always called first so malformed writes never hit disk.
func (r *Repository) Upsert(ctx context.Context, cfg *Config) error {
	if err := Validate(cfg); err != nil {
		return err
	}
	cfg.UpdatedAt = time.Now().UTC()
	return r.ctx(ctx).Save(cfg).Error
}

// UpdateEnabled flips the enabled bit for tenantID. Returns ErrNotFound
// if no row matches — which is how cross-tenant calls surface loudly
// (caller asked with wrong tenantID, got 0 RowsAffected).
func (r *Repository) UpdateEnabled(ctx context.Context, tenantID uuid.UUID, enabled bool) error {
	res := r.ctx(ctx).
		Model(&Config{}).
		Where("tenant_id = ?", tenantID).
		Updates(map[string]any{
			"enabled":    enabled,
			"updated_at": time.Now().UTC(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes the config and every JIT mapping for a tenant, in a
// single transaction. Mappings go first so a partial failure never
// leaves orphaned bindings pointing at a deleted provider.
func (r *Repository) Delete(ctx context.Context, tenantID uuid.UUID) error {
	return r.ctx(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Where("tenant_id = ?", tenantID).
			Delete(&UserMapping{}).Error; err != nil {
			return err
		}
		res := tx.
			Where("tenant_id = ?", tenantID).
			Delete(&Config{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// UpsertMapping writes (or refreshes) a JIT user binding. last_login_at
// is always stamped with the current time — this is the hot path the
// callback handler hits on every successful SSO login.
func (r *Repository) UpsertMapping(ctx context.Context, m *UserMapping) error {
	if m == nil || m.TenantID == uuid.Nil || m.ExternalUserID == "" || m.InternalUserID == uuid.Nil {
		return fmt.Errorf("%w: mapping missing tenant_id/external_user_id/internal_user_id", ErrInvalidMetadata)
	}
	now := time.Now().UTC()
	m.LastLoginAt = &now
	m.UpdatedAt = now
	return r.ctx(ctx).Save(m).Error
}

// GetMapping fetches a single mapping by (tenant_id, external_user_id).
func (r *Repository) GetMapping(ctx context.Context, tenantID uuid.UUID, externalID string) (*UserMapping, error) {
	var m UserMapping
	err := r.ctx(ctx).
		Where("tenant_id = ? AND external_user_id = ?", tenantID, externalID).
		First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// FindMappingByEmail finds a JIT mapping by tenant + email (citext
// comparison in Postgres makes this case-insensitive).
func (r *Repository) FindMappingByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*UserMapping, error) {
	var m UserMapping
	err := r.ctx(ctx).
		Where("tenant_id = ? AND email = ?", tenantID, email).
		First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}
