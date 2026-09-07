package domain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/domainvalidate"
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

// SecretScope identifies a single token entry. The scope is hashed into
// the GCP Secret Manager resource name by the SecretStore so a token is
// uniquely addressable per (tenant, domain). Mirrors the carriersecrets
// scope without forcing this package to import that one.
type SecretScope struct {
	TenantID string
	Domain   string // category bucket — fixed to "platform" for CF tokens.
	Provider string // fixed to "cloudflare".
	Field    string // sanitized FQDN — one secret per merchant domain.
}

// SecretStore is the minimal surface needed by the domain service to
// store and resolve the merchant's Cloudflare API token. Production
// wires in the carriersecrets HybridStore (GCP SM primary, inline
// encryptor fallback). Tests inject an in-memory stub.
//
// Token plaintext NEVER leaves this interface — Put accepts it and
// returns an opaque reference; Get takes a reference and returns the
// plaintext on demand. The caller persists only the reference.
type SecretStore interface {
	Put(ctx context.Context, scope SecretScope, plaintext string) (reference string, err error)
	Get(ctx context.Context, reference string) (plaintext string, err error)
	Destroy(ctx context.Context, reference string) error
}

// DNSResolver is the subset of net.Resolver that manual verification
// needs. Injected so the ownership proof is testable without real DNS.
type DNSResolver interface {
	LookupCNAME(ctx context.Context, host string) (string, error)
	LookupHost(ctx context.Context, host string) ([]string, error)
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// ServiceConfig groups dependencies for the domain service.
type ServiceConfig struct {
	DB          *gorm.DB
	Repo        Repository
	CF          CloudflareClient
	Provisioner Provisioner
	Secrets     SecretStore             // optional — falls back to inline-token storage if nil.
	Resolver    domainvalidate.Resolver // optional — net.DefaultResolver when nil.
	DNS         DNSResolver             // optional — net.DefaultResolver when nil.
	// ChallengeSecret keys the per-(tenant, domain) TXT token. Empty
	// disables the ownership proof, which is what local dev wants and
	// what config.Validate forbids in prod.
	ChallengeSecret string
	Logger          *slog.Logger
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
	db              *gorm.DB
	repo            Repository
	cf              CloudflareClient
	provisioner     Provisioner
	secrets         SecretStore
	resolver        domainvalidate.Resolver
	dns             DNSResolver
	challengeSecret string
	logger          *slog.Logger
}

func NewService(cfg ServiceConfig) *Service {
	dns := cfg.DNS
	if dns == nil {
		dns = net.DefaultResolver
	}
	return &Service{
		db:              cfg.DB,
		repo:            cfg.Repo,
		cf:              cfg.CF,
		provisioner:     cfg.Provisioner,
		secrets:         cfg.Secrets,
		resolver:        cfg.Resolver,
		dns:             dns,
		challengeSecret: cfg.ChallengeSecret,
		logger:          cfg.Logger,
	}
}

// ValidateDomain runs the public-facing existence check used by the
// inline UI validator and as a defense-in-depth gate inside Add.
// Returns the canonical lower-cased FQDN on success.
func (s *Service) ValidateDomain(ctx context.Context, raw string) (string, error) {
	return domainvalidate.Check(ctx, raw, s.resolver)
}

// scopeForCFToken returns the SecretScope that uniquely names the
// Cloudflare token entry for one (tenant, FQDN) pair. Field is the
// FQDN so multiple custom domains under the same tenant each get
// their own secret instead of stomping on a single shared entry.
func scopeForCFToken(tenantID uuid.UUID, fqdn string) SecretScope {
	return SecretScope{
		TenantID: tenantID.String(),
		Domain:   "platform",
		Provider: "cloudflare",
		Field:    fqdn,
	}
}

// List returns all custom domains for a store.
func (s *Service) List(ctx context.Context, storeID uuid.UUID) ([]CustomDomain, error) {
	return s.repo.List(ctx, s.db, storeID)
}

// RefreshCertStatus queries cert-manager for the current Certificate
// status and syncs cert_status + ssl_status on the DB row. Called when
// a merchant clicks "Refresh SSL status" in the admin UI.
func (s *Service) RefreshCertStatus(ctx context.Context, storeID, id uuid.UUID) (*CustomDomain, error) {
	d, err := s.repo.GetByID(ctx, s.db, storeID, id)
	if err != nil {
		return nil, err
	}
	if s.provisioner == nil || d.DNSMethod != DNSMethodManual {
		return d, nil
	}
	ready, message, err := s.provisioner.CertStatus(ctx, d.Domain)
	if err != nil {
		errMsg := err.Error()
		d.CertError = &errMsg
		d.CertStatus = "failed"
	} else if ready {
		d.CertStatus = "ready"
		d.SSLStatus = SSLStatusActive
		d.CertError = nil
	} else {
		d.CertStatus = "issuing"
		if message != "" {
			d.CertError = &message
		}
	}
	d.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, s.db, d); err != nil {
		return nil, err
	}
	return d, nil
}

// ResolveByDomain looks up a verified custom domain by FQDN and returns
// the associated store ID. Used by the storefront to route requests.
func (s *Service) ResolveByDomain(ctx context.Context, domainName string) (*CustomDomain, error) {
	return s.repo.GetByDomain(ctx, s.db, strings.ToLower(strings.TrimSpace(domainName)))
}

// AddInput holds the fields for adding a custom domain.
type AddInput struct {
	TenantID  uuid.UUID
	StoreID   uuid.UUID
	StoreSlug string
	Domain    string
	DNSMethod DNSMethod
	// CFAPIToken is the merchant's plaintext Cloudflare API token —
	// passed through Service.Add and immediately handed to the
	// SecretStore. It is never logged and never persisted to the DB.
	CFAPIToken string
}

// Add registers a new custom domain.
func (s *Service) Add(ctx context.Context, in AddInput) (*CustomDomain, error) {
	domainStr, vErr := s.ValidateDomain(ctx, in.Domain)
	if vErr != nil {
		// Map the sentinel onto our standard validation error so the
		// HTTP layer surfaces the user-friendly message verbatim.
		if errors.Is(vErr, domainvalidate.ErrInvalidDomain) {
			msg := strings.TrimPrefix(vErr.Error(), "invalid domain: ")
			return nil, apperrors.ValidationFailed("domain", msg)
		}
		return nil, vErr
	}

	method := in.DNSMethod
	if method == "" {
		method = DNSMethodManual
	}

	if method == DNSMethodCloudflare && strings.TrimSpace(in.CFAPIToken) == "" {
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

	// For the Cloudflare path, write the token to the SecretStore
	// BEFORE creating the row so a failure leaves no orphan record
	// pointing at a non-existent secret. The store is per-tenant by
	// construction; we never use a token written for tenant A to
	// authorise a CF call for tenant B.
	tokenRef := ""
	if method == DNSMethodCloudflare {
		if s.secrets == nil {
			return nil, errors.New("custom domains: secret store not configured — cloudflare method unavailable")
		}
		ref, putErr := s.secrets.Put(
			ctx,
			scopeForCFToken(in.TenantID, domainStr),
			strings.TrimSpace(in.CFAPIToken),
		)
		if putErr != nil {
			s.logger.Error("cloudflare token: secret store put failed",
				"tenant_id", in.TenantID, "domain", domainStr, "err", putErr)
			return nil, fmt.Errorf("store cloudflare token: %w", putErr)
		}
		tokenRef = ref
	}

	now := time.Now()
	d := &CustomDomain{
		TenantID:      in.TenantID,
		StoreID:       in.StoreID,
		Domain:        domainStr,
		DNSMethod:     method,
		CnameTarget:   &cnameTarget,
		Status:        DomainStatusVerifying,
		CFAPITokenRef: tokenRef,
		SSLStatus:     SSLStatusPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if method == DNSMethodCloudflare {
		d.Status = DomainStatusPending
	}

	if err := s.repo.Create(ctx, s.db, d); err != nil {
		// Roll back the secret so we don't leak orphan entries on a
		// duplicate-domain race or DB outage.
		if tokenRef != "" {
			if destroyErr := s.secrets.Destroy(context.Background(), tokenRef); destroyErr != nil {
				s.logger.Warn("cloudflare token: rollback destroy failed",
					"tenant_id", in.TenantID, "domain", domainStr, "err", destroyErr)
			}
		}
		return nil, err
	}

	if method == DNSMethodCloudflare && s.cf != nil {
		go s.registerWithCloudflare(d.ID, d.StoreID, domainStr, tokenRef)
	}

	return d, nil
}

func (s *Service) registerWithCloudflare(id, storeID uuid.UUID, domainStr, tokenRef string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	apiToken, err := s.resolveToken(ctx, tokenRef)
	if err != nil {
		s.logger.Error("cloudflare token: resolve failed", "domain", domainStr, "err", err)
		errMsg := "Could not retrieve the saved Cloudflare token. Re-add the domain with a fresh token."
		updated := &CustomDomain{
			ID:           id,
			StoreID:      storeID,
			Status:       DomainStatusError,
			ErrorMessage: &errMsg,
			UpdatedAt:    time.Now(),
		}
		_ = s.repo.Update(ctx, s.db, updated)
		return
	}

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
		// Best-effort cleanup of the CF DNS record. If we can't resolve
		// the token (e.g. secret already destroyed) we still remove the
		// row so the merchant isn't stuck — the orphan record on
		// Cloudflare is a minor concern compared to a stuck takedown.
		if apiToken, tokErr := s.resolveToken(ctx, d.CFAPITokenRef); tokErr == nil && apiToken != "" {
			if cfErr := s.cf.RemoveDomain(ctx, *d.CloudflareZoneID, *d.CloudflareDNSRecordID, apiToken); cfErr != nil {
				s.logger.Error("cloudflare remove domain failed", "domain", d.Domain, "err", cfErr)
			}
		} else if tokErr != nil {
			s.logger.Warn("cloudflare remove: token unresolvable, skipping CF API call",
				"domain", d.Domain, "err", tokErr)
		}
	}

	// Tear down k8s resources for manual domains — best effort.
	if s.provisioner != nil && d.DNSMethod == DNSMethodManual {
		if err := s.provisioner.Deprovision(ctx, d.Domain); err != nil {
			s.logger.Error("k8sprov deprovision failed", "domain", d.Domain, "err", err)
		}
	}

	if err := s.repo.Delete(ctx, s.db, storeID, id); err != nil {
		return err
	}

	// Best-effort destroy of the CF token secret AFTER the row is gone
	// so a Destroy failure doesn't leave the merchant with a row that
	// has no resolvable token. Idempotent on the GSM side.
	if d.DNSMethod == DNSMethodCloudflare && s.secrets != nil && d.CFAPITokenRef != "" {
		if destroyErr := s.secrets.Destroy(ctx, d.CFAPITokenRef); destroyErr != nil {
			s.logger.Warn("cloudflare token: destroy after row delete failed",
				"domain", d.Domain, "err", destroyErr)
		}
	}

	return nil
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

	if err := s.requireChallenge(ctx, d); err != nil {
		return s.failVerify(ctx, d, err.Error())
	}

	target := *d.CnameTarget
	cname, cnameErr := s.dns.LookupCNAME(ctx, d.Domain)
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
	merchantIPs, merchantErr := s.dns.LookupHost(ctx, d.Domain)
	targetIPs, targetErr := s.dns.LookupHost(ctx, target)

	if merchantErr == nil && targetErr == nil && len(merchantIPs) > 0 && len(targetIPs) > 0 {
		if ipSetsOverlap(merchantIPs, targetIPs) {
			return s.markVerified(ctx, d)
		}
	}

	// Neither CNAME match nor A-record match — produce the most useful
	// error we can, favouring the CNAME reason when present.
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
	return s.failVerify(ctx, d, errMsg)
}

// Challenge returns the TXT record the merchant must publish to prove
// ownership, and whether it is still outstanding. Exposed so the admin
// UI can show the record instead of the merchant guessing.
func (s *Service) Challenge(d *CustomDomain) (host, token string, required bool) {
	if s.challengeSecret == "" || d.VerifiedAt != nil {
		return "", "", false
	}
	return ChallengeHost(d.Domain), ChallengeToken(s.challengeSecret, d.TenantID, d.Domain), true
}

// requireChallenge proves the merchant controls the domain's DNS zone,
// which routing alone does not: a CNAME to our edge, and especially a
// shared CDN edge IP, is something anyone can point at us. Domains
// verified before this check existed are grandfathered so a refresh
// cannot take a live storefront down.
func (s *Service) requireChallenge(ctx context.Context, d *CustomDomain) error {
	host, want, required := s.Challenge(d)
	if !required {
		return nil
	}
	records, err := s.dns.LookupTXT(ctx, host)
	if err != nil || !ChallengeMatches(records, want) {
		return fmt.Errorf("Ownership not proven. Add a TXT record at %s with the value %s, then verify again.", host, want)
	}
	return nil
}

func (s *Service) failVerify(ctx context.Context, d *CustomDomain, msg string) (*CustomDomain, error) {
	d.Status = DomainStatusVerifying
	d.ErrorMessage = &msg
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
	//
	// Both DNS methods need provisioning: cert-manager + Istio is what
	// actually serves HTTPS for the merchant's domain. Cloudflare-auto
	// only handled the CNAME — TLS still terminates at our ingress, so
	// this Provision call must run regardless of how DNS got there.
	if s.provisioner != nil {
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

	apiToken, tokErr := s.resolveToken(ctx, d.CFAPITokenRef)
	if tokErr != nil {
		errMsg := "Could not retrieve the saved Cloudflare token. Re-add the domain with a fresh token."
		d.Status = DomainStatusError
		d.ErrorMessage = &errMsg
		d.UpdatedAt = time.Now()
		_ = s.repo.Update(ctx, s.db, d)
		return d, nil
	}

	verified, sslActive, cfErr := s.cf.VerifyDomain(ctx, *d.CloudflareZoneID, d.Domain, apiToken)
	if cfErr != nil {
		errMsg := cfErr.Error()
		d.Status = DomainStatusError
		d.ErrorMessage = &errMsg
		d.UpdatedAt = time.Now()
		_ = s.repo.Update(ctx, s.db, d)
		return d, nil
	}

	// Once Cloudflare confirms the CNAME exists, hand off to markVerified
	// so the Cloudflare-auto path issues the cert + Gateway exactly the
	// same way the manual path does. SSL termination always happens at
	// our ingress, regardless of who created the DNS record.
	if verified {
		out, err := s.markVerified(ctx, d)
		if err != nil {
			return nil, err
		}
		if sslActive {
			out.SSLStatus = SSLStatusActive
			_ = s.repo.Update(ctx, s.db, out)
		}
		return out, nil
	}

	now := time.Now()
	d.Status = DomainStatusVerifying
	if sslActive {
		d.SSLStatus = SSLStatusActive
	}
	d.UpdatedAt = now
	if err := s.repo.Update(ctx, s.db, d); err != nil {
		return nil, err
	}
	return d, nil
}

// resolveToken returns the plaintext Cloudflare API token for a domain.
// Routes the reference through the SecretStore when one is configured,
// or treats the value as a legacy inline token when no store is wired
// (dev mode, or rows created before this refactor). Empty refs are
// rejected with a clear error so callers don't silently call CF with
// an empty bearer header.
func (s *Service) resolveToken(ctx context.Context, ref string) (string, error) {
	if ref == "" {
		return "", errors.New("custom domains: no Cloudflare token on file")
	}
	if s.secrets == nil {
		// Pre-refactor rows held the plaintext token here. Pass it
		// through unchanged so we don't break boot for dev setups
		// that don't wire a secret store.
		return ref, nil
	}
	plain, err := s.secrets.Get(ctx, ref)
	if err != nil {
		return "", err
	}
	if plain == "" {
		return "", errors.New("custom domains: stored token is empty")
	}
	return plain, nil
}
