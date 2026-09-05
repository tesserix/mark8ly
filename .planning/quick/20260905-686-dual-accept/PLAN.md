# Mobile admin: accept both issuers during the migration (#686)

## Why this first

`ZITADEL_ENABLED` on `marketplace-api-admin` is an ATOMIC switch of two things:
the bearer verifier, and the source of `tenant_id` (GIP claim vs FGA-validated
`X-Acting-Tenant-Id`). One verifier is mounted, never both. So the day it flips,
every already-installed app stops working — there is no drain window, and the
mobile cutover becomes a flag day.

EAS history confirms real builds exist and predate the crash fixed in #719, so
those installs authenticate against GIP today and must keep working.

## The second reason, which is the actual #708 blocker

Nobody can currently tell whether any traffic still authenticates via GIP. Without
that, "GIP is safe to delete" is a guess. A composite verifier is the natural place
to answer it, because it is the one point that knows which issuer a token came from.

## Tasks

### 1. `CompositeVerifier` (TDD)
Ordered list of verifiers; first success wins; returns the first error when all
fail. Records which issuer succeeded.
**Done when:** unit tests cover — Zitadel token accepted, GIP token accepted,
garbage rejected, first-error surfaced, and the issuer label recorded per outcome.

### 2. Per-token tenancy, replacing the per-deployment switch
`GIPBearerAuth` writes `tenant_id` from the claim only when the claim is NON-EMPTY.
Zitadel tokens carry no claim, so they fall through to `TenantFromRequest`, which
is then safe to mount alongside: it never aborts, and it only writes after an FGA
membership check. Ordering (bearer -> TenantFromRequest) means a VALIDATED value
can only ever overwrite an unvalidated one, never the reverse — which is the
invariant the current mutual exclusion exists to protect.
**Done when:** tests prove a GIP token still resolves its claim tenant, a Zitadel
token resolves via the header, and a GIP token PLUS header resolves to the
FGA-validated header value.

### 3. Metric
`mobile_admin_token_verified_total{issuer}`. This is what makes the GIP drain
observable and #708 decidable.
**Done when:** the counter increments with the right label for each issuer.

### 4. Wiring + config
A flag selecting single vs dual verifier. Adding required config is k8s-first;
this must default to today's behaviour so the deploy order is safe.
**Done when:** `go build`, `go vet`, and the full package tests pass, and the
default path is byte-for-byte today's behaviour.

## Out of scope
The mobile app's OIDC/PKCE flow and sending `X-Acting-Tenant-Id`. This change is
what makes that shippable without a flag day; it does not do it.
