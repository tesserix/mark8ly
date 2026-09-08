# A route manifest, so a deleted endpoint goes red (#834 tasks 3-4)

## The failure this must catch

#826 deleted the `arbitrage-appeal` endpoint, the `arbitrage_flag` field and its
table. `apps/admin` kept a page, a form, an API proxy, a banner, a client, hooks
and Zod schemas pointing at it, and **every gate stayed green**: `tsc` passed
because the frontend types were internally consistent, `vitest` passed because
the units were mocked, `next build` passed, and the e2e spec passed by mocking
the endpoint that had just been deleted. It was found by grep during cleanup.

## Two corrections to the original plan, measured on current main

**1. The `page.route()` half is MOOT.** The original plan said covering the e2e
mock globs "goes red on six real files the moment you turn it on". #851 deleted
five of those six specs and the sixth went with them:

```
grep -rn "page.route" apps/*/tests/e2e/*.ts apps/*/tests/operator/*.ts  ->  0 files
```

There is nothing left to check. Building that check would be guarding an empty
set — the exact species of dead scaffolding this issue is about. Dropped, and
recorded rather than silently omitted.

**2. `func main()` cannot be called, so the manifest is per-subtree.** Route
wiring lives inside `func main()` from `cmd/marketplace-api/main.go:311` to
~`:3080`. A test cannot invoke it. The 19 subtree mounts are at `:2591-2759`.

## The design, taken from two instruments this repo already has

Neither half is invented, and the split between them is the point.

**Routes come from the real gin trees, never from parsing.**
`internal/handlers/admin/route_parity_test.go:293-311` builds `gin.New()`, calls
`RegisterAdmin(engine.Group("/api/v1"), deps)` and reads `engine.Routes()`. Its
docblock (`:36-44`) explicitly rejects source-parsing and hand-kept route lists
as "an anti-pattern that already bit this codebase once". It also has
`fillHandlers` (`:344`), which reflects over the Deps struct filling nil handler
pointers — without it, every `if deps.X != nil` route is invisible.

**Mounting is checked by AST, because it cannot be called.**
`cmd/marketplace-api/wiring_test.go` already parses `main.go` with `go/parser`
for exactly this reason. That is not a contradiction of the above: gin answers
*which routes a subtree registers*, the AST answers *whether main.go mounts that
subtree*. Neither question is answered by a hand-kept list.

**Reconciliation is bidirectional**, as in
`internal/handlers/platformadmin/conformance_declaration_test.go:324-386`, which
reports "MOUNTED but NOT DECLARED" and "DECLARED but NOT MOUNTED" as distinct
failures. A one-directional check lets a deletion pass silently, which is #826.

## Scope, and why it is defensible

Frontend-facing only: `/api/v1/admin`, `/api/v1/mobile/admin`,
`/api/v1/storefront`, `/api/v1/mobile/storefront`, `/api/v1/platform`,
`/api/v1/public`.

`/internal` (44 groups) is excluded because **no frontend references it** —
verified: every `/internal` hit in `apps/admin` and `apps/storefront` is a Go
source path in a comment, and the one that looks like a call
(`lib/auth/serverSession.ts:197`) is prose in a docblock with no `fetch`. The
scope boundary is itself assertable, and task 3 asserts it, so the exclusion
cannot rot into a blind spot.

## Tasks

### 1. The manifest and its bidirectional test
- [ ] A test in `cmd/marketplace-api` that builds each in-scope subtree on a
      fresh `gin.New()`, reads `engine.Routes()`, and reconciles `METHOD PATH`
      pairs against a committed `route-manifest.json`.
- [ ] Reuse `route_parity_test.go`'s `fillHandlers` rather than reimplementing
      it — conditionally-registered routes are invisible without it, and a
      manifest missing them would be confidently wrong.
- [ ] Both directions named distinctly: mounted-but-undeclared (someone added a
      route) and declared-but-unmounted (someone deleted one — this is #826).
- [ ] Assert the scope boundary: no manifest entry begins `/internal`.
- **Done when** deleting a handler fails the build, adding one fails until the
  manifest is regenerated, and the manifest diff shows
  `- POST /api/v1/admin/.../arbitrage-appeal` as a review signal on its own.

### 2. Prove it catches #826, by reproducing it
- [ ] Delete a real route in a scratch commit, show the test naming it, revert.
      Not a synthetic fixture — an actual route from the manifest.
- [ ] Also verify the opposite direction: add a route, show the test failing as
      undeclared.
- **Done when** both directions are demonstrated with real output. A guard that
  has not been watched failing is not known to work — three separate defects in
  #851's canary were only found by making it fail on purpose.

### 3. The frontend check
- [ ] Extract the literal backend paths `apps/admin` references and assert each
      resolves against the manifest. Measured on current main: **106** distinct
      `${MARKETPLACE_API_URL}` paths plus **16** literal `proxyAdminApi` call
      sites.
- [ ] **Classify by base-variable resolution, not path prefix.**
      `components/products/media/mediaUploadClient.ts` writes
      `${MARKETPLACE_API_URL}/api/admin/...` with no `/api/v1` — that is
      correct, because the base resolves to `""` in the browser and the request
      lands on a Next proxy route. A prefix-based extractor reports it as drift.
- [ ] Follow `proxyAdminApi` and the three `*Url()` suffix helpers one hop.
- **Done when** a path the backend does not serve fails CI, and the
  media-upload case does NOT produce a false positive.

## Limits, stated now rather than discovered later

- **Method drift stays invisible** if the check is path-only: the frontend's
  method sits in an adjacent object literal or the `apiClient.get/post`
  selector. Dropping `PATCH /coupons/:id` while keeping `GET` would pass. Assert
  method too where cheaply available; say so where not.
- **Response-shape drift is out of reach entirely.** #826's `arbitrage_flag` was
  a *field*. A route check catches the deleted endpoint, never a deleted field.
  That needs Zod schemas reconciled against Go DTO tags — a separate instrument
  and its own issue, not a silent gap in this one.
- **`/internal` is unguarded** by construction. Justified today because no
  frontend reaches it; task 1's boundary assertion is what keeps that true.
