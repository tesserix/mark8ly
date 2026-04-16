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
	CF          CloudflareClient
	Provisioner Provisioner
	Logger      *slog.Logger
}

// Provisioner is the k8s provisioning interface consumed by Service.
// Implemented by internal/k8sprov.Provisioner. Kept as an interface so
// the service can run without cluster access (local dev, tests) and so
// the k8sprov import doesn't pull into every unit test.
type Provisioner interface {
	Provision(ctx context.Context, domain string) (*ProvisionResult, error)
	Deprovision(ctx context.Context, domain string) error
	CertStatus(ctx context.Context, domain string) (ready bool, message string, err error)
}

// ProvisionResult mirrors k8sprov.ProvisionResult so the domain package
// doesn't import k8sprov (avoids cycles, keeps deps narrow).
type ProvisionResult struct {
	CertSecretName string
}

// Service implements custom domain CRUD operations.
type Service struct {
	db          *gorm.DB
	repo        Repository
	cf          CloudflareClient
	provisioner Provisioner
	logger      *slog.Logger
}

func NewService(cfg ServiceConfig) *Service {
	return &Service{
		db:          cfg.DB,
		repo:        cfg.Repo,
		cf:          cfg.CF,
		provisioner: cfg.Provisioner,
		logger:      cfg.Logger,
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

	if in.StoreSlug == "" {
		return nil, apperrors.ValidationFailed("store_slug", "store slug is required to build CNAME target")
	}
	// edge.mark8ly.com is a DNS-only A record pointing to our
	// custom-ingressgateway public IP (not proxied through Cloudflare).
	// Merchants CNAME their domain here so TLS terminates at our gateway
	// where the per-domain cert lives — Cloudflare edge doesn't have
	// certs for arbitrary merchant domains, so we bypass CF entirely.
	cnameTarget := "edge.mark8ly.com"
	_ = in.StoreSlug

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

	// Tear down k8s resources for manual domains — best effort.
	if s.provisioner != nil && d.DNSMethod == DNSMethodManual {
		if err := s.provisioner.Deprovision(ctx, d.Domain); err != nil {
			s.logger.Error("k8sprov deprovision failed", "domain", d.Domain, "err", err)
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
	cname, cnameErr := net.DefaultResolver.LookupCNAME(ctx, d.Domain)
	cname = strings.TrimSuffix(cname, ".")

	// Happy path: a real CNAME that matches target (or resolves via a
	// chain that ends at target). This handles non-apex subdomains and
	// any registrar that serves a true CNAME.
	if cnameErr == nil && strings.EqualFold(cname, target) {
		return s.markVerified(ctx, d)
	}

	// ALIAS/ANAME fallback. Apex domains can't have CNAME per DNS spec,
	// so registrars like Hostinger, Cloudflare (via flattening), DNSimple,
	// and Netlify serve ALIAS/ANAME — which resolves to A records at
	// lookup time. Compare A-record sets: if every IP that the merchant's
	// domain resolves to is also an IP that the target resolves to,
	// accept it. We also accept the reverse (target is a subset of
	// merchant IPs) since CDN edges can return subsets over time.
	merchantIPs, merchantErr := net.DefaultResolver.LookupHost(ctx, d.Domain)
	targetIPs, targetErr := net.DefaultResolver.LookupHost(ctx, target)

	if merchantErr == nil && targetErr == nil && len(merchantIPs) > 0 && len(targetIPs) > 0 {
		if ipSetsOverlap(merchantIPs, targetIPs) {
			return s.markVerified(ctx, d)
		}
	}

	// Neither CNAME match nor A-record match — produce the most useful
	// error we can, favouring the CNAME reason when present.
	d.Status = DomainStatusVerifying
	var errMsg string
	switch {
	case cnameErr != nil && merchantErr != nil:
		errMsg = fmt.Sprintf("No DNS records found for %s. Add a CNAME (or ALIAS / ANAME if your provider supports it) pointing to %s.", d.Domain, target)
	case cnameErr != nil && len(merchantIPs) > 0:
		errMsg = fmt.Sprintf("%s resolves to %s but should resolve to %s. Update your DNS record to point to the correct target.", d.Domain, strings.Join(merchantIPs, ", "), target)
	case cname != "" && !strings.EqualFold(cname, target):
		errMsg = fmt.Sprintf("CNAME points to %s, expected %s", cname, target)
	default:
		errMsg = fmt.Sprintf("DNS not ready yet. Ensure %s points to %s.", d.Domain, target)
	}
	d.ErrorMessage = &errMsg
	d.UpdatedAt = time.Now()
	_ = s.repo.Update(ctx, s.db, d)
	return d, nil
}

// ipSetsOverlap returns true when at least one IP appears in both sets.
// Used to accept ALIAS records where both domains resolve to the same
// CDN edge IPs.
func ipSetsOverlap(a, b []string) bool {
	set := make(map[string]struct{}, len(b))
	for _, ip := range b {
		set[ip] = struct{}{}
	}
	for _, ip := range a {
		if _, ok := set[ip]; ok {
			return true
		}
	}
	return false
}

func (s *Service) markVerified(ctx context.Context, d *CustomDomain) (*CustomDomain, error) {
	now := time.Now()
	d.Status = DomainStatusActive
	d.VerifiedAt = &now
	d.ErrorMessage = nil
	// DNS is confirmed; SSL provisioning begins below. Keep SSLStatus as
	// pending until cert-manager reports Ready=True.
	d.SSLStatus = SSLStatusPending
	d.UpdatedAt = now

	// Kick off k8s provisioning: Certificate + Gateway + VirtualService.
	// Best-effort — failures log but don't fail the verify since DNS is
	// already confirmed. A background poller can retry later.
	if s.provisioner != nil && d.DNSMethod == DNSMethodManual {
		if res, err := s.provisioner.Provision(ctx, d.Domain); err != nil {
			s.logger.Error("k8sprov provision failed", "domain", d.Domain, "err", err)
			msg := "SSL provisioning error: " + err.Error()
			d.CertError = &msg
			d.CertStatus = "failed"
		} else {
			d.CertSecretName = &res.CertSecretName
			d.CertStatus = "issuing"
			d.CertError = nil
		}
	}

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
