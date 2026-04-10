package domain

import (
	"context"
	"fmt"
	"log/slog"
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
	// AddDomain registers a domain in Cloudflare and returns zone ID + DNS record ID.
	AddDomain(ctx context.Context, domain string, apiToken string) (zoneID, dnsRecordID string, err error)

	// RemoveDomain tears down the Cloudflare resources for a domain.
	RemoveDomain(ctx context.Context, zoneID, dnsRecordID, apiToken string) error

	// VerifyDomain checks whether the domain's DNS is correctly pointed.
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

// NewService constructs a domain Service.
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

// AddInput holds the fields for adding a custom domain.
type AddInput struct {
	TenantID            uuid.UUID
	StoreID             uuid.UUID
	Domain              string
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
	if in.CFAPITokenEncrypted == "" {
		return nil, apperrors.ValidationFailed("cf_api_token", "Cloudflare API token is required")
	}

	now := time.Now()
	d := &CustomDomain{
		TenantID:            in.TenantID,
		StoreID:             in.StoreID,
		Domain:              domainStr,
		Status:              DomainStatusPending,
		CFAPITokenEncrypted: in.CFAPITokenEncrypted,
		SSLStatus:           SSLStatusPending,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if err := s.repo.Create(ctx, s.db, d); err != nil {
		return nil, err
	}

	// Kick off Cloudflare registration asynchronously if client is available.
	if s.cf != nil {
		go s.registerWithCloudflare(d.ID, d.StoreID, domainStr, in.CFAPITokenEncrypted)
	}

	return d, nil
}

// registerWithCloudflare runs in the background after Add.
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

	// Tear down Cloudflare resources if they exist.
	if s.cf != nil && d.CloudflareZoneID != nil && d.CloudflareDNSRecordID != nil {
		if cfErr := s.cf.RemoveDomain(ctx, *d.CloudflareZoneID, *d.CloudflareDNSRecordID, d.CFAPITokenEncrypted); cfErr != nil {
			s.logger.Error("cloudflare remove domain failed", "domain", d.Domain, "err", cfErr)
			// Continue with DB deletion — CF cleanup can be retried.
		}
	}

	return s.repo.Delete(ctx, s.db, storeID, id)
}

// Verify checks domain DNS and SSL status via Cloudflare.
func (s *Service) Verify(ctx context.Context, storeID, id uuid.UUID) (*CustomDomain, error) {
	d, err := s.repo.GetByID(ctx, s.db, storeID, id)
	if err != nil {
		return nil, err
	}

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
