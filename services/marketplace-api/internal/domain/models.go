// Package domain implements custom domain CRUD for Settings S2.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// DomainStatus enumerates the lifecycle states of a custom domain.
type DomainStatus string

const (
	DomainStatusPending   DomainStatus = "pending"
	DomainStatusVerifying DomainStatus = "verifying"
	DomainStatusActive    DomainStatus = "active"
	DomainStatusError     DomainStatus = "error"
	DomainStatusRemoving  DomainStatus = "removing"
)

// DNSMethod indicates how DNS is managed for the custom domain.
type DNSMethod string

const (
	DNSMethodManual     DNSMethod = "manual"
	DNSMethodCloudflare DNSMethod = "cloudflare"
)

// SSLStatus enumerates the SSL provisioning states.
type SSLStatus string

const (
	SSLStatusPending  SSLStatus = "pending"
	SSLStatusActive   SSLStatus = "active"
	SSLStatusError    SSLStatus = "error"
	SSLStatusInactive SSLStatus = "inactive"
)

// CustomDomain is the GORM model for the custom_domains table.
type CustomDomain struct {
	ID                    uuid.UUID    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID              uuid.UUID    `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID               uuid.UUID    `gorm:"column:store_id;type:uuid;not null"`
	Domain                string       `gorm:"column:domain;type:varchar(253);not null;uniqueIndex"`
	DNSMethod             DNSMethod    `gorm:"column:dns_method;type:varchar(20);not null;default:manual"`
	CnameTarget           *string      `gorm:"column:cname_target;type:varchar(253)"`
	Status                DomainStatus `gorm:"column:status;type:varchar(20);not null;default:pending"`
	CloudflareZoneID      *string      `gorm:"column:cloudflare_zone_id;type:varchar(100)"`
	CloudflareDNSRecordID *string      `gorm:"column:cloudflare_dns_record_id;type:varchar(100)"`
	CFAPITokenEncrypted   string       `gorm:"column:cf_api_token_encrypted;type:text;default:''"`
	SSLStatus             SSLStatus    `gorm:"column:ssl_status;type:varchar(20);not null;default:pending"`
	VerifiedAt            *time.Time   `gorm:"column:verified_at"`
	ErrorMessage          *string      `gorm:"column:error_message;type:text"`
	// Cert provisioning state. Populated when k8sprov creates a
	// cert-manager Certificate resource for the domain.
	CertStatus     string  `gorm:"column:cert_status;type:varchar(20);not null;default:pending"`
	CertSecretName *string `gorm:"column:cert_secret_name;type:varchar(253)"`
	CertError      *string `gorm:"column:cert_error;type:text"`
	CreatedAt             time.Time    `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt             time.Time    `gorm:"column:updated_at;not null;default:now()"`
}

func (CustomDomain) TableName() string { return "custom_domains" }
