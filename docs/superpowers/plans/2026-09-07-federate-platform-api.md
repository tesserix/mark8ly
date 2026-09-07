# Federate platform-api — implementation plan

**Issue:** #720
**Unblocks:** the auth-template half of tesserix/tesserix-home#588 and #586;
also #330, whose decision is contingent on Task 1 (see
`specs/2026-09-07-console-otto-routing.md`).

## Verified starting state

Read, not assumed:

| Fact | Evidence |
|---|---|
| platform-api has **no** contract surface | Only two comments mention `platformadmin` (`internal/account/handler.go:92`, `internal/tenant/suspend_handler_test.go:140`). Its only prefix is `v1 := r.Group("/api/v1")`, `cmd/server/main.go:411` |
| The three files to extract have **zero** marketplace-api imports | `signature.go`, `nonce.go`, `middleware.go` import only stdlib + `uuid`, `gorm`, `gin` |
| `audit.go` is the only coupled file | imports `internal/audit` — it **stays behind** |
| A `go.work` already spans the 4 modules | `go.work` |
| Nonce table already specified | `platform_request_nonces`, marketplace-api `000101_platform_admin_audit.up.sql:27` |
| platform-api's migrations are 4-digit | latest `0015_stores_suspended_by_tenant` |

## The constraint that shapes the whole plan

**Every Go image builds with `context: "services/<svc>"` and a bare
`COPY . .`** (`.github/ci/container-images.json`). A sibling module is
outside the build context, so a `replace ../../packages/platformauth` would
pass `go build` locally — `go.work` covers it — and **fail in Docker**. The
local green is not evidence.

Decision: move the two Go images to `context: "."`, matching what all three
Next.js images already do. This is why Task 2 exists and why it is separate
from Task 1: the extraction is not done when the code compiles, it is done
when the image builds.

**Kargo forms Freight only when all 7 mark8ly images share a new tag.** A
Dockerfile that builds locally but fails in CI stalls every deploy, not just
this one. Task 2 ends with an actual `docker build`, not a `go build`.

---

### Task 1: extract `packages/platformauth`

Create a 5th module, `github.com/mark8ly/platformauth`, holding
`signature.go`, `nonce.go`, `middleware.go` and their tests plus
`testdata/vectors.json`.

- `go.work`: add `./packages/platformauth`.
- `services/marketplace-api/go.mod`: `require` + `replace ../../packages/platformauth`.
- Rewrite marketplace-api's references. `audit.go` stays in `platformadmin`
  and reads the exported context keys (`CtxOperatorID`, `CtxCapability`),
  which must be exported from the new module.
- **`.github/workflows/ci.yml`: add `platformauth` to the `go` matrix.**
  Without this the moved tests never run again and CI stays green while
  coverage silently drops — the failure mode #720 names explicitly.

**Done when:** the `go` matrix job named `Go (platformauth)` appears in a CI
run and its test count is non-zero. Not when `go test ./...` exits 0 locally
— that proves the code moved, not that CI followed it.

The golden vectors are the contract the console signs against. If a vector
file moves, its path must move with it in the same commit; a test that
silently skips because `testdata/` is missing looks identical to a pass.

### Task 2: repo-root build context for the two Go images

- `.github/ci/container-images.json`: `context` → `"."` for
  `mark8ly-platform-api` and `mark8ly-marketplace-api`. Leave `source_root`
  pointing at the service (it drives path filtering, not the build).
- Rewrite both Dockerfiles' `COPY` paths for the new root:
  `COPY services/<svc>/go.mod services/<svc>/go.sum ./`, the `packages/`
  tree, and `WORKDIR` / build target paths.
- `auth-bff` and `otto` are **not** touched in this task. They gain the
  module only when they need it (otto in #330).

**Done when:** `docker build` succeeds locally for both, from the repo root,
against the pinned base digests — and the resulting binary runs `/health`.

Watch for: the base images are pinned by GHCR digest and the weekly rebuild
prunes old ones. A build failing in ~20s at `load metadata` is that, not this
diff.

### Task 3: mount the contract surface in platform-api

- `require` + `replace` the new module.
- Migration `0016_platform_request_nonces` — port
  marketplace-api's `000101` table into platform-api's own database. The
  nonce store is per-service state; sharing a table across services is not
  on the table because they hold separate databases.
- Mount under **`/api/v1/platform`**, never `/api/v1/admin`. An Istio
  AuthorizationPolicy in `istio-ingress` denies un-JWT'd requests to
  `/api/v1/admin`, and this surface authenticates by HMAC, so the mesh
  answers `403 RBAC: access denied` before the application sees it. It
  reproduces in neither local dev nor CI — see
  `marketplace-api/internal/handlers/platformadmin/routes.go:225-245`.
- Follow marketplace-api's refuse-to-mount discipline: writes must not mount
  without an audit emitter, and a missing config answers `503 not_configured`
  rather than mounting an unauthenticated route.

**Done when:** the negative control passes — a made-up path under the same
prefix answers **404** while a real route unsigned answers **401**. A 404 on
both means the surface was never mounted, which is this codebase's recurring
silent failure and is indistinguishable from success in a smoke test that
only asserts "not 200".

### Task 4: register as a federation source, and answer the slug question

In tesserix's platform-api and `tesserix-k8s`:

- Its own signing secret, and `FEDERATION_*` env for the new base URL.
- A registry entry.
- **Answer the one-slug-per-product question in the registry's own
  comments.** `federation.Product.Slug` is documented as carrying the value
  `console-core`'s `EstateProduct.context` carries. Two slugs for what an
  operator calls one product needs a deliberate answer.

  This plan's recommendation: a **service map under one product slug**, not a
  second product — the same shape #330 commits Otto to. `mark8ly` gains
  `marketplace-api` and `platform-api` entries; Otto becomes a third later.
  Shape the map to hold a third entry now, so #330 does not have to migrate it.

- **Ordering:** adding required config is a **k8s-first** change. The env and
  secret land before the code that reads them, or the service crash-loops on
  boot. Removing config is the opposite order.

**Done when:** a signed request from tesserix platform-api reaches mark8ly
platform-api in the cluster. Not when the env var exists.

### Task 5: serve email templates on the new surface

Match the shape marketplace-api's surface landed with in #588 so the console
can treat both halves uniformly, the way the old UI's
`platform_api` / `marketplace_api` toggle does. platform-api owns `welcome`,
`email_verification`, `invitation`, `password_reset`, `login_otp`,
`new_device_login` (`platform-api/migrations/0013`).

**Done when:** the console editor lists auth templates alongside order and
billing ones.

---

## Sequencing

1 → 2 → 3 are strictly ordered; 2 gates 3 because a surface that cannot ship
in an image is not mounted. 4 is k8s-first and can start once 3's routes are
known. 5 is last and independently reviewable.

## What this plan deliberately does not do

- **No change to `auth-bff` or `otto`.** They gain the module when they need
  it. Touching four images to serve two is how a middleware extraction turns
  into an estate-wide deploy stall.
- **No shared nonce table.** Separate databases, separate tables.
- **No new auth mechanism.** The surface speaks the existing signing scheme
  or it is not this issue.
