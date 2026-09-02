# Moving per-tenant carrier credentials from GCP Secret Manager to OpenBao

Design for [#319]. Supersedes nothing; the transport question that issue left
open is answered here and recorded on the issue itself.

## Problem

Per-tenant payment, shipping and tax credentials — the Razorpay secret key a
merchant types into admin settings, the Delhivery API key, the TaxJar token —
live in **GCP Secret Manager**. The estate is standardising on **OpenBao**,
which now runs as a 3-node HA cluster with Kubernetes auth, KV v2 and snapshot
backups. mark8ly is the outlier.

The database never stores plaintext. It stores an opaque reference
(`gsm://projects/{PROJECT}/secrets/{NAME}`) that `internal/carriersecrets`
resolves. Three domains are in play, not two:

| Domain | Call sites | Examples |
| --- | --- | --- |
| `payment` | 6 | `settings.go:555`, `checkout_ext.go:132`, `webhooks.go:147` |
| `shipping` | 7 | `settings.go:722`, `shipping_rates.go:350`, `shipments.go:326` |
| `tax` | 2 | `settings.go:1444`, `checkout_ext.go:214` |

All three share one `Store`, so migrating the store migrates all three. `tax` is
not extra work, but it is not optional either.

## Why this is a port, not a redesign

`internal/carriersecrets` is unusually well shaped for a backend swap, and the
package **has already done this once** — from inline `noop:/aes:` ciphertext to
`gsm://` references, using the same mechanism proposed here.

- `store.go:48` — `Store` is an interface. Callers hold **opaque** references.
- `store.go:90` — `HybridStore` already implements primary-plus-read-fallback.
- `store.go:196` — `MaybeRewrap` already migrates a reference on next save.
- **15 non-test files** reference the package.

The load-bearing property is that **`Get` routes on the reference prefix**.
References are self-describing, so dual-read is a consequence of the data model
rather than a mode flag bolted on top.

## Decisions

Each was taken explicitly. The alternative and its cost are recorded so a later
reader can tell a decision from an accident.

### D1. Transport: direct to OpenBao via Kubernetes ServiceAccount

marketplace-api authenticates to OpenBao with its own ServiceAccount token.
`secret-service` stays the human console and does **not** grow a machine API.

*Rejected:* routing through `secret-service`. It would give one audited
chokepoint, but the audit record we actually want is OpenBao's own, which we get
either way — so it would add a hop, not a record. It would also put a
human-facing console on the checkout request path, an availability dependency
that does not exist today. These secrets are also per-tenant and minted at
runtime, which is not the static, operator-curated shape `secret-service` and
ESO are built around.

### D2. Migration: lazy rewrap **and** an active backfill

*Rejected:* lazy rewrap alone. It is the smaller change and it is what the
package did last time, but a merchant who never re-saves their key never
migrates — so the fallback counter never reaches zero and GCP SM can never be
retired. Lazy-only and "decommission GCP SM" are mutually exclusive; the issue
proposed both.

*Rejected:* big-bang backfill with no lazy path. Largest blast radius, no
fallback for a credential that fails to migrate.

### D3. Sequencing: five phases, grant proven first

See Phases below. Each phase is independently verifiable and revertible.

### D4. Read resilience: short-TTL cache, stale-on-error

60s TTL. Two consequences, stated rather than buried:

- Decrypted credentials sit in process memory for up to 60s.
- A rotation takes up to 60s to take effect.

*Rejected:* no cache. Most secure and rotation is instant, but an OpenBao blip
becomes a checkout outage with no cushion.

### D5. Port the OpenBao client; do not extract to `go-shared` yet

`secret-service/apps/api/internal/bao/client.go` is production-quality: mutex-
guarded lazy re-login with a one-minute expiry margin, health check, and error
translation (404→`ErrNotFound`, 403→`ErrForbidden`, KV v2 CAS races). Port it to
`mark8ly/internal/bao`.

*Rejected for now:* extracting to `go-shared`. Two consumers do not justify a
shared-library release plus a dependency bump across ~30 services. Revisit at a
third consumer.

## Reference format and path model

```
bao://kv/mark8ly/marketplace-api/tenants/<tenantID>/<domain>/<provider>/<field>
```

A new self-describing prefix alongside `gsm://` and `noop:/aes:`.

**No version in the reference.** KV v2 gives "same path, new version", so a
rotation returns the identical reference. This preserves the property #319 calls
out as load-bearing: rotation must not require rewriting references across the
DB. `Get` always resolves latest.

**No `<env>` segment**, corrected during planning. GCP needed a
`mark8ly-prod` / `mark8ly-test` prefix because both environments shared one GCP
project. OpenBao runs *in cluster*, and the estate's convention is that the path
prefix is the namespace name (`kv/data/homechef/*`, `kv/data/devai/devai-api/*`).
Environment separation is therefore inherent — a different environment is a
different cluster, a different OpenBao, and a different namespace. An `<env>`
segment would be redundant and off-convention.

### The grant: two ServiceAccounts, three pod identities

marketplace-api runs as **two** ServiceAccounts, not one, and both resolve
credentials:

| ServiceAccount | Runs | Credential use |
| --- | --- | --- |
| `mark8ly-marketplace-api-admin` | admin engine | settings saves (`Put`) |
| `mark8ly-marketplace-api-storefront` | storefront engine | checkout, shipping rates, payment webhooks (`Get`) |

**`namespaceWhitelist` is the wrong mechanism here**, corrected during
planning. It auto-generates a **read-only** policy at
`kv/data/<namespace>/<app>/*` — built for ESO-style consumers that only read.
We need `create`/`update` (a merchant saving a key) and `delete` on metadata
(`Destroy`), on a per-tenant path. Using the whitelist would grant read-only
access to the wrong path and fail on the first save.

Instead this uses explicit `bootstrap.policies` and `bootstrap.kubernetesRoles`
entries. Policy names deliberately avoid the `app-` prefix: the secret-service
console owns that prefix at runtime and the bootstrap Job never reconciles it,
so an unprefixed name keeps the two from fighting over the same policy.

**The CronJobs need no additional OpenBao role.** `refund-sweep` and the other
marketplace-api CronJobs run under `serviceAccountName: {{ .Values.name }}` —
that is, `mark8ly-marketplace-api-admin` — so the admin grant already covers
them. This is why phase 4's OpenBao authorisation is free.

**But they still need their own NetworkPolicy entry.** The OpenBao ingress
policy matches on namespace *plus* `app.kubernetes.io/name`, and a CronJob's
pods carry a distinct label (`mark8ly-marketplace-api-admin-refund-sweep`), not
the deployment's. This is precisely the trap that broke #587: identical
ServiceAccount, different pod label, silent connection timeout rather than a
refusal. Phase 4 must add the label, and prove it with a live run.

### Policy

**Two policies, least privilege.** Every `Put` and every `Destroy` in the
codebase is admin-side (`handlers/admin/settings.go:553,720,1442` and
`internal/domain/service.go:292,394`); storefront handlers only read. So the
storefront engine gets no write capability at all:

```hcl
# policy: mark8ly-marketplace-api-admin-carrier-secrets
path "kv/data/mark8ly/marketplace-api/tenants/*"     { capabilities = ["create","read","update"] }
path "kv/metadata/mark8ly/marketplace-api/tenants/*" { capabilities = ["read","list","delete"] }

# policy: mark8ly-marketplace-api-storefront-carrier-secrets
path "kv/data/mark8ly/marketplace-api/tenants/*"     { capabilities = ["read"] }
path "kv/metadata/mark8ly/marketplace-api/tenants/*" { capabilities = ["read","list"] }
```

`Destroy` deletes the **metadata** path, removing all versions — matching
today's GCP `Destroy`. KV v2's soft delete would leave recoverable plaintext.

### What this does not buy

**OpenBao will not enforce tenant isolation.** marketplace-api authenticates as
a single ServiceAccount and therefore holds a wildcard over every tenant path.
Isolation is enforced in application code by `Scope.TenantID`, exactly as today —
the GCP secret ID already embeds the tenant, and a wrong-scope leak is equally
possible now. This is **not a regression and not an improvement**, and the spec
says so because #319 is labelled `area:security` and it would be easy to imply
otherwise. Per-tenant policies would require per-tenant authentication, which
the application has no way to perform.

The real gains are an audit log of every credential read, and one backend
instead of two.

## Components

| Component | Purpose |
| --- | --- |
| `internal/bao/client.go` | Ported OpenBao client. K8s login against the role for the pod's ServiceAccount (see Grant below), token cached and renewed before expiry. |
| `internal/carriersecrets/bao.go` | `BaoStore` over KV v2 at the path above. |
| `internal/carriersecrets/chain.go` | `ChainStore` — `Get` routes by prefix, `Put` to primary, implements `Rewrapper`. |
| `internal/carriersecrets/cache.go` | Decorator. 60s TTL, stale-on-error. Separate so it is independently testable and omittable in tests. |
| `internal/carriersecrets/fakebao.go` | Mirrors the existing `FakeClient`; handler tests need no network. |
| `cmd/carrier-secrets-backfill` | Phase 3 job, dry-run capable. |

The `Store` interface is **unchanged**, so all 15 call sites are untouched. That
is the point of the ChainStore approach.

*Rejected:* nesting a new hybrid around `HybridStore` (two layers of rewrap
semantics), and extending `HybridStore` in place (it already carries four
GCP-shaped fields; phase 5 would mean surgery on the live production store).

### Metrics — what makes phase 5 decidable

- `carriersecrets_gsm_fallback_total` — a `gsm://` resolution while Bao is primary.
- `carriersecrets_cache_hits_total`, `carriersecrets_cache_stale_served_total`.

Without the first counter, "the fallback counter has been durably zero" is
unmeasurable and GCP SM never gets retired on evidence.

## Failure behaviour

**Writes never silently fall back.** If OpenBao is unavailable, a merchant's save
fails with a clear error. A silent write fallback would mint `gsm://` references
after cutover and make the counter a lie.

**A `bao://` reference never falls back to GCP.** The value is not there. Serve
stale from cache if available, otherwise fail closed with a typed error.

**Reads do not gate readiness.** Putting OpenBao health into `/ready` was
considered and rejected: with `gsm://` rows still resolving via GCP, an OpenBao
blip would be amplified into a total checkout outage. Loud structured logs and
metrics instead. The lesson being applied is "make silent failures loud", not
"make partial failures total".

## Phases

Each ends with a **live run**, not a green test suite.

| # | Scope | Repo | Live verification |
| --- | --- | --- | --- |
| 1 | Grant: `namespaceWhitelist` (both SAs), `security.yaml` destinations, NetworkPolicy | tesserix-k8s | `scope-probe` Job round-trips a dummy path |
| 2 | `ChainStore`, `BaoStore`, cache, client; **Bao primary for new writes** | mark8ly | One real credential saved and read back in prod |
| 3 | Backfill | mark8ly | Dry-run counts reconcile against a DB count of `gsm://` rows; then real run |
| 4 | `WithSecretStore` in `refund-sweep-cron` (+ its NetworkPolicy label; the role is already covered) | both | A real sweeper run resolving a credential |
| 5 | Retire GCP SM | both | Fallback counter durably zero across a stated window |

Phase 1 exists because **cross-chart grants fail silently**. #587 demonstrated
this twice in one day: a CronJob whose entrypoint was missing from the image,
and a CronJob denied by a NetworkPolicy in a different chart from the one
defining it. Both reviewed clean. Only a live run found them.

Phase 2 puts Bao on the smallest self-healing surface — only newly saved or
rotated credentials — and rolls back by flipping one config value, with
already-written `bao://` rows still resolving.

Phase 4 also closes #166's live blocker: `cmd/refund-sweep-cron/main.go:50`
calls `orderrefund.NewResolver(conn)` and never calls the existing
`WithSecretStore`, so the sweeper cannot resolve `gsm://` references and every
gateway re-drive from it fails. It may be pulled forward if refunds need
unblocking sooner; because the OpenBao role is already covered by the admin
ServiceAccount, the only extra cost is its NetworkPolicy label.

## Testing

- Table-driven prefix routing, including the unknown-prefix case.
- Cache TTL expiry and stale-on-error.
- `Destroy` removes all versions.
- `FakeBao` for handler tests.
- Integration tests against a real OpenBao; `TEST_DATABASE_URL`-style gating so
  a missing backend **skips loudly rather than passing silently**.

Every new test is verified to fail before it passes.

## Out of scope

`internal/breakglass/` and `internal/arbitrage/` also use GCP Secret Manager,
but they are operator secrets with different rotation and access stories. They
move separately, if at all.

[#319]: https://github.com/tesserix/mark8ly/issues/319
