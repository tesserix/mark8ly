# How the console reaches Otto for cross-tenant live chat

**Status:** decided
**Decides:** #330
**Depends on:** #720 (federating platform-api) — see §5
**Series:** platform console integration (#274)

**Decision: option (c).** Otto implements the platform signing scheme
directly, and the console's federation registry gains a per-product **service
map** rather than a flat `admin_api_base`. Otto is reached as
`mark8ly/otto`, not as a separate product and not through a
`marketplace-api` proxy.

---

## 1. Why the original recommendation no longer holds

#330 leaned toward **(b)** — proxy Otto behind `marketplace-api` — for one
stated reason: *"the contract's one-base-URL assumption is load-bearing for
the console's registry."*

That premise is gone. #720 requires the console to reach mark8ly's
`platform-api`, which is a **second base URL for the same product** and holds
the auth half of the email-template registry. The registry has to model one
product with several service endpoints whether or not Otto ever exists.

So (b) now buys nothing it used to buy. It only adds a hop.

## 2. What the code actually constrains

Verified by reading it, not inferred from the issue.

### 2.1 Otto's admin surface is store-scoped by middleware

```go
// services/otto/cmd/server/main.go:128
admin := r.Group("/api/v1/admin/otto")
admin.Use(auth.StaffAuth(cfg.InternalAuthSecret), auth.StoreResolver())
```

`StoreResolver` (`internal/auth/store.go:62`) requires a resolved store on
every request. A console operator triaging across tenants has no store.

**This is the load-bearing fact: every option needs a new, unscoped surface in
Otto.** (b) does not avoid the work — it adds a proxy in front of it. Once
Otto must grow a cross-tenant surface regardless, the only question left is
which auth that surface carries, and HMAC is the one the console already
speaks.

### 2.2 Otto's realtime leg already bypasses proxies

```go
// services/otto/cmd/server/main.go:177 — WebSocket on a no-middleware group
// "Istio routes /api/v1/otto/.../ws directly to Otto, bypassing the
//  Next.js proxies that would otherwise inject our auth headers.
//  Ticket auth takes over instead."
adminWS := r.Group("/api/v1/admin/otto")
adminHandler.RegisterWS(adminWS)
```

Live chat is not a REST surface with some polling bolted on; it is a
WebSocket surface with a REST control plane. The merchant admin already
proved a proxy cannot carry the realtime leg and split it: REST through the
proxy, WS direct with a short-lived ticket minted over the REST leg
(`POST /ws-ticket`, `admin_handler.go:82`).

Choosing (b) for the console therefore does not produce "one base URL". It
produces one base URL for the REST half and a direct Otto connection for the
realtime half — the split (b) exists to avoid, with a proxy still in the
middle of the other half.

### 2.3 Operator identity already crosses the boundary

`internal/ottoclient/forward.go:88-104` forwards `X-Internal-Auth` plus
`X-User-Id` / `X-User-Email` / `X-User-Name` / `X-Client-Tenant-Id`. #330's
hardest requirement — *a message must be attributable to the human who sent
it, not to "the platform"* — is a solved shape in this estate, not a new one.
Under (c) it is carried by the signed `Operator` field the platform scheme
already defines (`platformadmin/signature.go`), which is strictly better:
attribution becomes part of what is signed rather than a header a hop could
drop.

Precedent for the failure being avoided: mobile's IDP provider is pinned in
three services and **two of the three hops drop it invisibly**. Identity that
rides in an unsigned header across a proxy is exactly that bug waiting to
recur.

## 3. Why not (a) — Otto as its own product

`console-core`'s `ESTATE` is keyed on product, and an operator thinks of Otto
as part of mark8ly, not beside it. Registering `otto` as a peer of `mark8ly`
makes the registry disagree with the operator's mental model to encode a
deployment detail. (c) keeps the product boundary where the human boundary is
and lets the registry carry the service split, which is a deployment fact.

## 4. What (c) costs, stated honestly

Otto gains an HMAC-authenticated surface it does not have today. That is real
work, and it is the one advantage (b) held.

It is affordable **only because of #720**, which extracts the
signature/nonce/auth middleware out of `marketplace-api/internal/handlers/
platformadmin` into a shared Go module. Otto is a fourth Go module in this
repo with no shared package today; after #720 it consumes the same module
`platform-api` does. Without #720 this decision would be wrong, because it
would mean a third hand-rolled implementation of signature verification.

**If #720 is abandoned, reopen this decision.** It is contingent, and the
contingency is written down rather than left to be rediscovered.

## 5. Sequencing

1. **#720 lands** — shared middleware module, and the registry answers the
   one-slug-per-product question with a service map (#720 acceptance item 4).
   The map must be shaped to hold a third entry, not just the two #720 needs.
2. Otto mounts a cross-tenant surface under `/api/v1/platform` — never
   `/api/v1/admin`, for the Istio reason in
   `platformadmin/routes.go:225-245`: an AuthorizationPolicy in
   `istio-ingress` denies un-JWT'd requests to `/api/v1/admin`, and this
   surface authenticates by HMAC, so the mesh answers `403 RBAC: access
   denied` before the application sees the request. It reproduces in neither
   local dev nor CI.
3. Registered as `mark8ly/otto` in the service map, with its own secret.
4. WS stays direct with ticket auth, unchanged from the merchant pattern.
   The ticket is minted over the signed REST leg.

## 6. Still blocked on the console, and that is fine

`platform.liveChat` is `pending: true` in
`packages/console-core/src/routes.ts:194`, with a test asserting it
(`routes.console.test.ts:237`). Nothing here asks the console to build sooner.

What changes is that the console is no longer blocked *on us for an answer*.
When it picks the surface up, the transport is decided and the registry shape
it needs is the one #720 is already building.

## 7. Estate-wide or mark8ly-scoped

The old admin pointed both the mark8ly and Fe3dr rails at the same
`/admin/support/live-chat`, so the queue was cross-product by design. Under
(c) that stays true and costs nothing extra: a second product's Otto is
another service-map entry under that product. The console surface is
estate-wide; this decision is about how mark8ly's half of it is reached.
