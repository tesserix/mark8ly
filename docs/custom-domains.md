# Custom Domains — Runbook

End-to-end guide for the self-service custom domain feature that lets merchants serve their storefront and admin from their own domain (e.g. `shop.mybrand.com` / `admin.mybrand.com`) with valid Let's Encrypt SSL.

## Merchant experience

1. Admin → Settings → Custom domains → add domain (e.g. `mybrand.com`)
2. Copy the DNS records shown in the admin UI:
   - **A `@` → `34.14.139.74`** — routes apex traffic to our custom-ingressgateway
   - **CNAME `_acme-challenge` → `_acme-challenge.mybrand.com.acme.mark8ly.com`** — delegates SSL issuance to us
   - **A `admin` → `34.14.139.74`** — optional, enables `admin.mybrand.com`
3. Paste at their DNS provider (any registrar — Hostinger, GoDaddy, Namecheap, Route53, Cloudflare, etc.)
4. Click **Verify** in admin
5. After ~1 min, SSL is live at `https://mybrand.com` and `https://admin.mybrand.com`

The ACME CNAME delegation means merchants never share DNS credentials with us — we issue certs in our `mark8ly.com` Cloudflare zone, they just delegate one subdomain.

## What happens automatically on Verify

`services/marketplace-api/internal/domain/service.go` → `markVerified()` calls `k8sprov.Provision()` which creates four k8s resources via the dynamic client:

| Kind | Namespace | Name | Purpose |
|---|---|---|---|
| `Certificate` (cert-manager.io/v1) | `istio-ingress` | `<slug>-tls` | Requests Let's Encrypt cert via DNS-01 through `letsencrypt-custom-domain` ClusterIssuer |
| `Gateway` (networking.istio.io/v1) | `istio-ingress` | `<slug>-gateway` | Terminates TLS on the `custom-ingressgateway` with the cert secret |
| `VirtualService` (networking.istio.io/v1) | `mark8ly` | `<slug>-route` | Routes `admin.<domain>` → admin app, everything else → storefront |
| `AuthorizationPolicy` (security.istio.io/v1) | `istio-ingress` | `allow-custom-domain-<slug>` | Allowlists the hosts on the custom-ingressgateway (otherwise 403) |

Remove → `k8sprov.Deprovision()` deletes all four.

## Architecture

```
Browser
  ↓ HTTPS (TLS terminates here at our cluster, not Cloudflare)
34.14.139.74 (custom-ingressgateway LoadBalancer in istio-ingress ns)
  ↓ Istio Gateway (per-domain, references cert secret)
  ↓ AuthorizationPolicy (allow rule per domain)
  ↓ VirtualService (host-based routing)
  ├─ admin.<domain> → mark8ly-admin:4202
  └─ default        → mark8ly-storefront:4203
```

The **storefront** resolves the slug: `lib/slug.ts` → `resolveStoreSlug()` → `slugFromHost()` (for `*.mark8ly.com`) OR `resolveCustomDomain()` API call (for custom domains).

The **admin** resolves the tenant: `apps/admin/middleware.ts` matches `{slug}-admin.mark8ly.com` OR `admin.<custom-domain>` and calls `/api/v1/storefront/resolve-domain` to get the slug, then switches session tenant accordingly.

## Infrastructure dependencies

| Resource | Where | Purpose |
|---|---|---|
| `34.14.139.74` | Istio `custom-ingressgateway` LoadBalancer (istio-ingress ns) | Public IP merchants point their A record to |
| `edge.mark8ly.com` | Cloudflare DNS record (DNS-only, not proxied) | Alias for `34.14.139.74`. CNAME target option in admin UI |
| `letsencrypt-custom-domain` | cert-manager ClusterIssuer | LE prod issuer with DNS-01 via Cloudflare |
| `cloudflare-api-token` | Secret in `cert-manager` ns, synced from GCP Secret Manager `prod-cloudflare-api-token` via ExternalSecrets (1h refresh) | CF API token cert-manager uses to write TXT records in the mark8ly.com zone |
| `mark8ly-marketplace-api-custom-domains` ClusterRole + bindings | `tesserix-k8s/charts/apps/mark8ly-marketplace-api-admin/templates/custom-domains-rbac.yaml` | Grants marketplace-api SAs permission to CRUD Certificate + Gateway + VirtualService + AuthorizationPolicy |

## Database

`custom_domains` table (marketplace-api DB):
- `dns_method` — `manual` (A/CNAME → our gateway) or `cloudflare` (unused; stub remains)
- `cname_target` — `edge.mark8ly.com` (shown to merchants as CNAME option)
- `status` — `pending` | `verifying` | `active` | `error`
- `ssl_status` — `pending` | `active`
- `cert_status` — `pending` | `issuing` | `ready` | `failed`
- `cert_secret_name` — points to the k8s secret with the cert
- `cert_error` — latest error from cert-manager
- `error_message` — latest DNS verification error

## Operations

### Rotate the Cloudflare API token

**Why**: if the token leaks or we want tighter scope.

**Prerequisites**: CF dashboard access + `gcloud` CLI with write perms on `tesseracthub-480811` project.

1. Cloudflare Dashboard → My Profile → API Tokens → Create Token → Custom Token
   - Permissions: `Zone → DNS → Edit`
   - Zone Resources: `Include → Specific zone → mark8ly.com`
   - TTL: blank (no expiration)
2. Update GCP Secret Manager:
   ```bash
   echo -n 'NEW_TOKEN_HERE' | gcloud secrets versions add prod-cloudflare-api-token --data-file=-
   ```
3. Force ExternalSecrets to resync immediately (otherwise waits up to 1h):
   ```bash
   kubectl annotate externalsecret cloudflare-api-token -n cert-manager force-sync="$(date +%s)" --overwrite
   ```
4. Verify:
   ```bash
   kubectl get secret cloudflare-api-token -n cert-manager -o jsonpath='{.data.api-token}' | base64 -d
   ```
5. Revoke old token in CF dashboard

### Debug a stuck cert

```bash
# Overview
kubectl get certificate <slug>-tls -n istio-ingress

# Challenge state (DNS-01 issuance)
kubectl describe challenges.acme.cert-manager.io -n istio-ingress | grep -B1 -A3 <domain>

# Check the TXT record exists (cert-manager should create this)
dig TXT _acme-challenge.<domain>.acme.mark8ly.com +short

# Common errors:
#   "9109: Invalid access token"            → CF token wrong/expired, rotate
#   "10502: Too many authentication failures" → CF rate-limit from past failures; wait ~1h
#   "Waiting for DNS-01 challenge propagation" → normal, wait 30–120s
```

### Force cert retry

```bash
# Delete the pending Order; cert-manager creates a fresh one
kubectl delete order -n istio-ingress <slug>-tls-1-XXXXXX
```

### Clean up a merchant's resources (outside normal remove flow)

```bash
SLUG=primasyss-com  # dots → hyphens
kubectl delete certificate,gateway.networking.istio.io,virtualservice.networking.istio.io,authorizationpolicy.security.istio.io -A -l custom-domain=$SLUG
kubectl exec -i -n mark8ly mark8ly-postgres-1 -c postgres -- psql -U postgres -d mark8ly_marketplace_api -c "DELETE FROM custom_domains WHERE domain = 'primasyss.com';"
```

### Change the gateway public IP

If `34.14.139.74` ever changes (LB recreated, region migration):

1. Update the `edge.mark8ly.com` A record in Cloudflare to the new IP
2. Update the hardcoded `34.14.139.74` in `apps/admin/components/settings/DomainsSettingsClient.tsx`
3. Tell existing merchants to update their A records — or temporarily run both IPs in parallel if possible

## Known limits / future work

- **No observability alerts** — cert issuance failures are silent. Follow-up: Prometheus + Alertmanager rule on `certmanager_certificate_ready_status{condition="True"} == 0 for 15m`.
- **No UI-side error surface for persistent cert failures** — admin shows `SSL pending` forever if DNS-01 keeps failing. Follow-up: backend flips `cert_status = failed` after N retries and admin surfaces it with action guidance.
- **Old domains** (added before the auto-provision code) have a VirtualService without the admin route. Remove + re-add to regenerate.
- **No Cloudflare SaaS / Custom Hostnames path** — if we ever want merchants' domains behind CF's DDoS/CDN edge, that requires the paid SaaS feature and a separate implementation.

## Source-of-truth files

| Concern | File |
|---|---|
| DNS verify + provisioning | `services/marketplace-api/internal/domain/service.go` |
| k8s resource templates | `services/marketplace-api/internal/k8sprov/client.go` |
| Admin HTTP handler | `services/marketplace-api/internal/handlers/admin/domains.go` |
| Storefront domain lookup | `apps/storefront/lib/slug.ts` |
| Admin tenant resolution | `apps/admin/middleware.ts` |
| Admin UI | `apps/admin/components/settings/DomainsSettingsClient.tsx` |
| RBAC (GitOps) | `tesserix-k8s/charts/apps/mark8ly-marketplace-api-admin/templates/custom-domains-rbac.yaml` |
| Migrations | `services/marketplace-api/migrations/000032_custom_domains_manual_method.up.sql` and `000034_custom_domains_cert_tracking.up.sql` |
