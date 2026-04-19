// Package sso implements §12 per-tenant SSO for Pro-tier tenants.
// Config + UserMapping here are the persistence shapes; SAML / OIDC
// protocol wiring lives in saml.go / oidc.go, and GIP tenant-pool
// provider management lives in gip_client.go.
package sso

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Provider is the SSO protocol flavour. Persisted as the
// sso_provider_kind ENUM in Postgres.
type Provider string

const (
	ProviderSAML Provider = "saml"
	ProviderOIDC Provider = "oidc"
)

// Valid returns true for recognised providers.
func (p Provider) Valid() bool { return p == ProviderSAML || p == ProviderOIDC }

// Config is a single tenant's SSO settings. Exactly one row per tenant
// (tenant_id is the primary key). `Metadata` is an opaque bag whose
// schema depends on `Provider`:
//
//   - SAML: idp_entity_id, idp_acs_url, idp_cert_pem, sp_entity_id, sp_acs_url
//   - OIDC: issuer, client_id, client_secret_ref (Secret Manager path),
//     discovery_url, redirect_uri, scopes
//
// Plaintext secrets NEVER live in metadata — client_secret_ref points
// at a Secret Manager path.
type Config struct {
	TenantID      uuid.UUID          `gorm:"column:tenant_id;type:uuid;primaryKey"`
	Provider      Provider           `gorm:"column:provider;type:sso_provider_kind;not null"`
	Metadata      datatypes.JSONMap  `gorm:"column:metadata;type:jsonb;not null"`
	AttrMapping   datatypes.JSONMap  `gorm:"column:attr_mapping;type:jsonb;not null;default:'{}'::jsonb"`
	GIPProviderID *string            `gorm:"column:gip_provider_id"`
	Enabled       bool               `gorm:"column:enabled;not null;default:false"`
	CreatedAt     time.Time          `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt     time.Time          `gorm:"column:updated_at;not null;autoUpdateTime"`
}

// TableName pins the Postgres table name.
func (Config) TableName() string { return "tenant_sso_configs" }

// UserMapping binds an external IdP identity (SAML NameID or OIDC sub)
// to an internal Mark8ly user within a single tenant. Composite PK
// (tenant_id, external_user_id); a second unique constraint guarantees
// one external identity per internal user per tenant.
type UserMapping struct {
	TenantID       uuid.UUID  `gorm:"column:tenant_id;type:uuid;primaryKey"`
	ExternalUserID string     `gorm:"column:external_user_id;primaryKey"`
	InternalUserID uuid.UUID  `gorm:"column:internal_user_id;type:uuid;not null"`
	Email          string     `gorm:"column:email;type:citext;not null"`
	LastLoginAt    *time.Time `gorm:"column:last_login_at"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;not null;autoUpdateTime"`
}

// TableName pins the Postgres table name.
func (UserMapping) TableName() string { return "tenant_sso_user_mappings" }

// Sentinel errors. Callers that need to distinguish missing configs
// from SQL errors should use errors.Is.
var (
	ErrNotFound        = errors.New("sso: not found")
	ErrInvalidProvider = errors.New("sso: invalid provider")
	ErrInvalidMetadata = errors.New("sso: invalid metadata")
	ErrInvalidAttrMap  = errors.New("sso: invalid attr_mapping")
)

// SAML metadata keys. Referenced from saml.go and validation.
const (
	SAMLKeyIDPEntityID = "idp_entity_id"
	SAMLKeyIDPACSURL   = "idp_acs_url"
	SAMLKeyIDPCertPEM  = "idp_cert_pem"
	SAMLKeySPEntityID  = "sp_entity_id"
	SAMLKeySPACSURL    = "sp_acs_url"
)

// OIDC metadata keys. Referenced from oidc.go and validation.
const (
	OIDCKeyIssuer          = "issuer"
	OIDCKeyClientID        = "client_id"
	OIDCKeyClientSecretRef = "client_secret_ref"
	OIDCKeyDiscoveryURL    = "discovery_url"
	OIDCKeyRedirectURI     = "redirect_uri"
	OIDCKeyScopes          = "scopes"
)
