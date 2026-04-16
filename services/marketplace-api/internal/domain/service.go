package domain

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// CloudflareClient abstracts Cloudflare API calls so the service can be
// tested without real CF credentials. Production wiring injects a real
// implementation; tests inject a stub.
type CloudflareClient interface {
	AddDomain(ctx context.Context, domain string, apiToken string) (zoneID, dnsRecordID string, err error)
	RemoveDomain(ctx context.Context, zoneID, dnsRecordID, apiToken string) error
	VerifyDomain(ctx context.Context, zoneID, domain string, apiToken string) (verified bool, sslActive bool, err error)
}

// ServiceConfig groups dependencies for the domain service.
type ServiceConfig struct {
	DB     *gorm.DB
	Repo   Repository
	CF     CloudflareClient
	Logger *slog.Logger
}

// Service implements custom domain CRUD operations.
type Service struct {
	db     *gorm.DB
	repo   Repository
	cf     CloudflareClient
	logger *slog.Logger
}

func NewService(cfg ServiceConfig) *Service {
	return &Service{
		db:     cfg.DB,
		repo:   cfg.Repo,
		cf:     cfg.CF,
		logger: cfg.Logger,
	}
}

// List returns all custom domains for a store.
func (s *Service) List(ctx context.Context, storeID uuid.UUID) ([]CustomDomain, error) {
	return s.repo.List(ctx, s.db, storeID)
}

// ResolveByDomain looks up a verified custom domain by FQDN and returns
// the associated store ID. Used by the storefront to route requests.
func (s *Service) ResolveByDomain(ctx context.Context, domainName string) (*CustomDomain, error) {
	return s.repo.GetByDomain(ctx, s.db, strings.ToLower(strings.TrimSpace(domainName)))
}

// AddInput holds the fields for adding a custom domain.
type AddInput struct {
	TenantID            uuid.UUID
	StoreID             uuid.UUID
	StoreSlug           string
	Domain              string
	DNSMethod           DNSMethod
	CFAPITokenEncrypted string
}

// Add registers a new custom domain.
func (s *Service) Add(ctx context.Context, in AddInput) (*CustomDomain, error) {
	domainStr := strings.ToLower(strings.TrimSpace(in.Domain))
	if domainStr == "" {
		return nil, apperrors.ValidationFailed("domain", "domain is required")
	}
	if len(domainStr) > 253 {
		return nil, apperrors.ValidationFailed("domain", "domain must be 253 characters or fewer")
	}

	method := in.DNSMethod
	if method == "" {
		method = DNSMethodManual
	}

	if method == DNSMethodCloudflare && in.CFAPITokenEncrypted == "" {
		return nil, apperrors.ValidationFailed("cf_api_token", "Cloudflare API token is required for cloudflare method")
	}

	cnameTarget := in.StoreSlug + ".mark8ly.com"

	now := time.Now()
	d := &CustomDomain{
		TenantID:            in.TenantID,
		StoreID:             in.StoreID,
		Domain:              domainStr,
		DNSMethod:           method,
		CnameTarget:         &cnameTarget,
		Status:              DomainStatusVerifying,
		CFAPITokenEncrypted: in.CFAPITokenEncrypted,
		SSLStatus:           SSLStatusPending,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if method == DNSMethodCloudflare {
		d.Status = DomainStatusPending
	}

	if err := s.repo.Create(ctx, s.db, d); err != nil {
		return nil, err
	}

	if method == DNSMethodCloudflare && s.cf != nil {
		go s.registerWithCloudflare(d.ID, d.StoreID, domainStr, in.CFAPITokenEncrypted)
	}

	return d, nil
}

func (s *Service) registerWithCloudflare(id, storeID uuid.UUID, domainStr, apiToken string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	zoneID, dnsRecordID, err := s.cf.AddDomain(ctx, domainStr, apiToken)
	if err != nil {
		s.logger.Error("cloudflare add domain failed", "domain", domainStr, "err", err)
		errMsg := err.Error()
		updated := &CustomDomain{
			ID:           id,
			StoreID:      storeID,
			Status:       DomainStatusError,
			ErrorMessage: &errMsg,
			UpdatedAt:    time.Now(),
		}
		if updateErr := s.repo.Update(ctx, s.db, updated); updateErr != nil {
			s.logger.Error("failed to update domain status", "domain", domainStr, "err", updateErr)
		}
		return
	}

	d, err := s.repo.GetByID(ctx, s.db, storeID, id)
	if err != nil {
		s.logger.Error("failed to get domain after CF registration", "id", id, "err", err)
		return
	}
	d.CloudflareZoneID = &zoneID
	d.CloudflareDNSRecordID = &dnsRecordID
	d.Status = DomainStatusVerifying
	d.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, s.db, d); err != nil {
		s.logger.Error("failed to update domain after CF registration", "domain", domainStr, "err", err)
	}
}

// Remove deletes a custom domain and tears down its Cloudflare resources.
func (s *Service) Remove(ctx context.Context, storeID, id uuid.UUID) error {
	d, err := s.repo.GetByID(ctx, s.db, storeID, id)
	if err != nil {
		return err
	}

	if d.DNSMethod == DNSMethodCloudflare && s.cf != nil && d.CloudflareZoneID != nil && d.CloudflareDNSRecordID != nil {
		if cfErr := s.cf.RemoveDomain(ctx, *d.CloudflareZoneID, *d.CloudflareDNSRecordID, d.CFAPITokenEncrypted); cfErr != nil {
			s.logger.Error("cloudflare remove domain failed", "domain", d.Domain, "err", cfErr)
		}
	}

	return s.repo.Delete(ctx, s.db, storeID, id)
}

// Verify checks domain DNS status. For manual domains, performs a DNS
// CNAME lookup. For Cloudflare domains, delegates to the CF client.
func (s *Service) Verify(ctx context.Context, storeID, id uuid.UUID) (*CustomDomain, error) {
	d, err := s.repo.GetByID(ctx, s.db, storeID, id)
	if err != nil {
		return nil, err
	}

	if d.DNSMethod == DNSMethodManual {
		return s.verifyManual(ctx, d)
	}
	return s.verifyCloudflare(ctx, d)
}

func (s *Service) verifyManual(ctx context.Context, d *CustomDomain) (*CustomDomain, error) {
	if d.CnameTarget == nil {
		return nil, apperrors.ValidationFailed("domain", "CNAME target not set — domain record is corrupted")
	}

	target := *d.CnameTarget
	cname, err := net.DefaultResolver.LookupCNAME(ctx, d.Domain)
	if err != nil {
		s.logger.Info("manual domain CNAME lookup failed", "domain", d.Domain, "err", err)
		d.Status = DomainStatusVerifying
		errMsg := fmt.Sprintf("CNAME record not found. Point %s → %s", d.Domain, target)
		d.ErrorMessage = &errMsg
		d.UpdatedAt = time.Now()
		_ = s.repo.Update(ctx, s.db, d)
		return d, nil
	}

	cname = strings.TrimSuffix(cname, ".")
	if !strings.EqualFold(cname, target) {
		d.Status = DomainStatusVerifying
		errMsg := fmt.Sprintf("CNAME points to %s, expected %s", cname, target)
		d.ErrorMessage = &errMsg
		d.UpdatedAt = time.Now()
		_ = s.repo.Update(ctx, s.db, d)
		return d, nil
	}

	now := time.Now()
	d.Status = DomainStatusActive
	d.VerifiedAt = &now
	d.ErrorMessage = nil
	d.SSLStatus = SSLStatusActive
	d.UpdatedAt = now
	if err := s.repo.Update(ctx, s.db, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *Service) verifyCloudflare(ctx context.Context, d *CustomDomain) (*CustomDomain, error) {
	if s.cf == nil {
		return nil, fmt.Errorf("cloudflare client not configured")
	}
	if d.CloudflareZoneID == nil {
		return nil, apperrors.ValidationFailed("domain", "domain has not been registered with Cloudflare yet")
	}

	verified, sslActive, cfErr := s.cf.VerifyDomain(ctx, *d.CloudflareZoneID, d.Domain, d.CFAPITokenEncrypted)
	if cfErr != nil {
		errMsg := cfErr.Error()
		d.Status = DomainStatusError
		d.ErrorMessage = &errMsg
		d.UpdatedAt = time.Now()
		_ = s.repo.Update(ctx, s.db, d)
		return d, nil
	}

	now := time.Now()
	if verified {
		d.Status = DomainStatusActive
		d.VerifiedAt = &now
		d.ErrorMessage = nil
	} else {
		d.Status = DomainStatusVerifying
	}

	if sslActive {
		d.SSLStatus = SSLStatusActive
	}

	d.UpdatedAt = now
	if err := s.repo.Update(ctx, s.db, d); err != nil {
		return nil, err
	}
	return d, nil
}
