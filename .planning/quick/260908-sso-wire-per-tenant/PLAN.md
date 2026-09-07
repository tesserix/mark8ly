# Wire per-tenant SSO (#820)

Both halves exist, are tested, and are registered behind a nil check that nothing
ever fills — so neither route exists at runtime. Same shape as the break-glass
login bug #642 was opened for.

- Config side: `NewSSOConfigHandler` is never called in `main.go`, so a merchant
  on the tier that pays for SSO has no way to configure it.
- Login side: `TenantResolver` has no implementation, `OIDCRPs` is never
  constructed, `loginSAML` is a stub.

## The one design change this needs

#820 suggests "build `OIDCRPs` at startup". **That would be wrong**, and the
handler's `map[string]*sso.OIDCRelyingParty` field is the reason it reads as
easy:

- `BuildOIDCRelyingParty` performs OIDC **discovery** — a network call to the
  tenant's issuer. Building the map at boot makes every tenant's IdP a startup
  dependency: one slow or unreachable IdP delays or fails the whole service.
- A map built at boot cannot contain a tenant who configures SSO afterwards. For
  a self-serve feature that means "works after the next deploy", which is not
  working.

So the map becomes a **lazily-populated cache**, built per tenant on first use
and invalidated when the config changes. Same handler behaviour, no startup
coupling, and a config saved at 10:00 works at 10:00.

## Tasks

### 1. TenantResolver over the store-slug projection
- [ ] Implement against `stores.SlugCache`, which already does exactly this
      lookup for admin/storefront routing, with a TTL and singleflight.
- [ ] Return `ErrTenantNotFound` on a miss so the handler answers 404 rather
      than 500.
- **Done when** `/sso/{slug}/login` resolves a real tenant and an unknown slug
  is a 404.

### 2. Lazy relying-party cache
- [ ] `sso.RelyingPartyCache` — build on demand, cache per tenant, TTL.
- [ ] Resolve `client_secret_ref` through OpenBao (`internal/bao`). The metadata
      comment still says "Secret Manager path"; that backend was retired in
      #813 and the comment is stale.
- [ ] Invalidate on config upsert/delete, so a corrected client secret takes
      effect without a restart.
- **Done when** a tenant configured after boot can log in, and a broken issuer
  fails that tenant's login rather than the service.

### 3. Mount both surfaces
- [ ] Construct `SSOConfigHandler` and fill `SSOLoginHandler`'s deps in
      `main.go`, on **both** engines.
- [ ] Mount tests driven through the real router. NOT a 404-vs-405 probe:
      `HandleMethodNotAllowed` is never set in this estate, so gin answers 404
      either way and that probe distinguishes nothing (#642, #820).

### 4. Stop `loginSAML` reporting success
- [ ] It currently returns **nothing** — a SAML tenant gets `200` with an empty
      body, which reads as success. The callback already answers `501` for the
      same reason. Make the two agree.
- **Done when** no SAML path can be mistaken for a working one.

## Open, and asked rather than assumed

Whether SAML should be implemented or removed outright. A full crewjam SP with
per-tenant middleware is its own project; a permanent `501` is honest but leaves
`sso_provider_kind` carrying a value nothing serves. Task 4 makes the current
state safe either way, and does not pre-empt the answer.

## Discovered mid-task: a fourth missing implementation

`sso.UsersRepo` has no implementation either — #820 lists three gaps and there
are four. JIT provisioning needs it for steps 3 and 4 of its binding algorithm:
"find the existing Mark8ly user with this email in this tenant", and "create
one".

Steps 1 and 2 run entirely off `tenant_sso_user_mappings`, so an SSO user who
has logged in before needs nothing new. The gap is the FIRST login.

And it cannot be satisfied locally. marketplace-api has no user identity store:
`user_profiles` is keyed on a provider subject string, is not tenant-scoped,
and holds display preferences. Membership is FGA tuples; identity is Zitadel.
Neither can answer "which user id has this email in this tenant" —
FGA checks a relation for a KNOWN id, and platform-api exposes no
lookup-by-email (`/users/me/tenants` and `/tenants/:id/me` both start from a
uid).

That makes step 3 an architectural decision rather than a wiring task, and the
wrong answer is silently wrong: a merchant admin who already signs in with a
password would, on their first SSO login, be minted a NEW internal id — a
second identity for one human, with the first one's role. Recorded on the
issue rather than guessed at here.
