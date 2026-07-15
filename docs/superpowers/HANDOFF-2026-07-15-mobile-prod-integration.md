# Handoff — mobile-admin prod integration (2026-07-15)

You are taking over work in `/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly` (Tesserix
"Mark8ly" — Go microservices + Next.js + an Expo mobile-admin app). Commit directly to `main`,
single-line conventional messages, no signatures, no PRs. Same convention for `tesserix-k8s`
(one level up: `/Users/Mahesh.Sangawar/personal/tesserix-new/tesserix-k8s`).

## Read first
`~/.claude/projects/-Users-Mahesh-Sangawar-personal-tesserix-new-mark8ly/memory/`
- **`istio_gip_issuer_not_configured.md`** — THE BLOCKER. Read before anything.
- **`mobile_admin_contract_mismatches.md`** — the 31 mismatches + locked decisions.
- `mobile_admin_nativewind_metro_landmines.md` — 19 build/runtime traps. Read before touching Expo.
- `project_mobile_admin_modernise.md` — program state, Apple config (all resolved).
- `MEMORY.md` — index.

## YOUR IMMEDIATE JOB — unblock mobile auth against prod

**The mobile app cannot authenticate against prod at all.** A *valid* GIP id_token gets
`HTTP 401 · Jwt issuer is not configured` from Envoy. That's Istio's JWT filter: no
`securetoken.google.com` issuer exists anywhere in the cluster — every RequestAuthentication still
points at Keycloak from before the GIP migration. Only mobile sends a Bearer JWT (web apps use
session cookies + HeaderTrustAuth), so it's the only client that trips it.

Independently corroborated: `tesserix-k8s/argocd/prod/infrastructure/homechef-ingress-gateway.yaml`
documents HomeChef hitting the identical wall — *"The shared gateway's Istio JWT filter rejects
anything that isn't a Keycloak JWT, which broke mobile login end-to-end."*

### What's already done (do NOT redo)
`tesserix-k8s@8ed38b1b` (pushed, ArgoCD synced) adds a `gip:` block to
`charts/infrastructure/istio-auth-policies` → renders `jwt-auth-gip` + `jwt-auth-gip-custom`.
**Additive: 93 insertions, 0 deletions.** FanZone Keycloak still renders (10 refs) + GIP (2).
Verified: policy live; selector `{"istio":"ingressgateway"}` matches `tesseract-gateway` (which
`mark8ly/mark8ly-wildcard` VS `*.mark8ly.com` routes through); issuer/aud exactly match the token;
**Envoy on `istio-ingressgateway` HAS the rule** (config_dump securetoken 0 → 2); gateway pod
reaches the JWKS (200). Keycloak rules are in that same Envoy (238 refs) so the push path works.

### THE OPEN QUESTION — ask the user first
**Prod still 401s.** Decisive test: `curl ...?probe=whichgw` then grep access logs of
`istio=ingressgateway`, `custom-ingressgateway`, `ingressgateway-internal` → **0 hits on all three**.
So `api.mark8ly.com` terminates somewhere else — or access logging is off on those gateways (which
would make that test blind; verify that possibility too).

**Ask the user: where does `api.mark8ly.com` actually terminate?** Candidates: a Cloudflare Worker
(CLAUDE.md: "Cloudflare Worker for routing instead of GCP LB"), a dashboard-managed Cloudflare tunnel
(no cloudflared ingress ConfigMap exists in-cluster), or a 4th proxy. Gateways in `istio-ingress`:
`istio-ingressgateway` (istio 1.29.1), `custom-ingressgateway`, `ingressgateway-internal`
(**NOT in `values.ingressGateways`** — could be the answer), `homechef-ingressgateway`.

Once known: either add that gateway to `values.ingressGateways`, or fall back to the **HomeChef
dedicated-gateway pattern** (labels that dodge the Keycloak selectors). Prefer adding the issuer —
HomeChef *had* to dodge because their Bearer is an AES-GCM blob, not a JWT; ours is a real JWT.

**GOTCHA:** `kubectl get gateway` resolves to the **Gateway API** type (ambient waypoints). Use
`kubectl get gateways.networking.istio.io -n istio-ingress`.

Verify the fix by re-running the real API call (recipe below) — do not declare victory on config alone.

## THEN — the contract work (blocked until auth works)

**31 verified mismatches.** The mobile API type layer was written speculatively ~2mo ago and never
run against prod. `client.ts` supports zod but **no module passes a schema**, so every mismatch is a
silent `undefined`. Blocker: `stores.go:74` returns `{"data":...}`, `use-store.ts` reads `.items` →
always `[]` → every merchant sees "No store yet" → **dashboard unreachable**.

Backend is PROVEN and NOT at fault — web admin drives the same handlers (`mobile_routes.go:21`:
"Same handlers, same authz, different auth"). **Except `/stores`**, which the web never calls (it
resolves store from the subdomain), so that one is unproven.

- **Spec for sub-project A committed: `docs/superpowers/specs/2026-07-15-mobile-admin-contract-foundation-design.md` (`2a042829`).** Approved by the user. Implementation plan NOT written — deliberately held until auth works, since A is unverifiable otherwise.
- Decomposition: **A** contract foundation (envelope/page-pagination/money/zod/`/stores`/dashboard) → **B** customers+dashboard fields → **C** orders → **D** products variant-aware → **E** backend gaps (Go+deploy).
- Locked: app adapts to backend; products variant-aware; types inferred via `z.infer`; money `z.union([number,string]).transform(Number)` (NOT `z.coerce` — turns null into 0; both wire forms are real); contract breaks fail loudly with the field path.
- **Key leverage:** wire-truthful schemas + inferred types turn the remaining mismatches into **compile errors** — B–D become "run tsc, fix what it names".

When A is unblocked: `superpowers:writing-plans` → `superpowers:subagent-driven-development`, then an
opus whole-branch review (`scripts/review-package BASE HEAD`). **Do not skip the final review** — it
has caught a Critical the per-task reviews missed on all three features this session, and verify its
claims yourself (one earlier finding was a false positive).

## Machine state / recipes

- **Metro:** real mode on :8081 (`npx expo start --dev-client --port 8081`, NO demo flag — a stale
  demo metro silently serves the demo bundle). May have died; restart from `apps/mobile-admin`.
- **Test creds** (tenant `MP-Internal-e986p`, Bondi store): `demo@mark8ly.com` / `Admin@123`.
  The `mahesh.sangawar@gmail.com/Admin@1234` in `e2e_test_state.md` is **STALE**.
- **Mint a token** (API key is public-by-design, in the gitignored plist):
  ```bash
  cd apps/mobile-admin
  KEY=$(plutil -extract API_KEY raw GoogleService-Info.plist)
  curl -s -X POST "https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=$KEY" \
    -H "Content-Type: application/json" \
    -d '{"email":"demo@mark8ly.com","password":"Admin@123","tenantId":"MP-Internal-e986p","returnSecureToken":true}' \
    | python3 -c "import sys,json;print(json.load(sys.stdin)['idToken'])"
  ```
  Then: `curl -H "Authorization: Bearer $T" https://api.mark8ly.com/api/v1/mobile/admin/stores`
  → currently `401 Jwt issuer is not configured`. **That call succeeding is the definition of done
  for the immediate job.** When it works, capture real response shapes for every endpoint and check
  them against the spec's hand-built fixtures.
- **Gates:** `cd apps/mobile-admin && npx jest` (**98/98**) · `npx tsc --noEmit 2>&1 | grep -c "error TS"` → **2** (pre-existing `_layout.tsx` expo-notifications; count, don't grep by filename — a per-file grep passed vacuously and missed 6 real errors) · demo `googleServicesFile` count must be 0.
- **NEVER** `npm ci` / `npm install` / `npm install --package-lock-only` / `rm -rf node_modules` — metro runs against this tree; `--package-lock-only` causes a 4871-line mass re-resolve.
- kubectl context is prod (`gke_tesseracthub-480811_asia-south1_tesseract-prod-in-gke`). ArgoCD sync
  nudge: `kubectl -n argocd annotate application <app> argocd.argoproj.io/refresh=hard --overwrite`.

## Completed this session — do NOT redo

1. **Connected accounts** (Settings→Security, `c3b7dd52..f448b42e`) — signed-in link/unlink, the Apple Hide-My-Email path. Reviewed, shipped.
2. **Auth error handling** (`68106ffb..ba3dbafb`, **98/98 jest**) — one firebase-free mapper (`packages/mobile-shared/auth/errors.ts`, zero imports), cancellation normalised at the native boundary, `auth/reauth-failed` tagging, `onUnauthorized(reason)` + `auth-notice` store, TenantGate 403 copy. Whole-branch review found the silent sign-out ALSO live in `support.tsx` (separate `SupportClientConfig`) — fixed.
3. **`^react$` jest pin** (`be32da8c`) — zustand is root-only and resolved the root's React 19.2.5 vs the app's 19.2.3 → any store-backed render crashed under jest. See landmine #18.
4. **Apple portal config — DONE.** Team `2CRHRRYBPL`, App ID `com.mark8ly.admin` (3 capabilities ticked), SIWA key `XZW4X8VKX4`, Services ID `com.mark8ly.admin.signin`, Apple provider enabled on tenant `MP-Internal-e986p`. APNs key `86FNRU7L3K` already exists team-scoped — **do NOT create a second** (hard cap 2/team). Bundle ID `com.mark8ly.admin` is LOCKED.

## Deferred / open
- **Apple sign-in device tap-through** — parked. iPhone paired, signing verified (`OU=2CRHRRYBPL`); `npx expo run:ios --device` is ready. `rawNonce: ""` in `signInWithAppleNative()` is the prime suspect if Apple linking fails.
- `keycloak.internal` in prod points at a **devtest** IdP with *mark8ly's* audiences → dead config, separate cleanup (deliberately not folded into 8ed38b1b).
- GIP REST returns `INVALID_PASSWORD` (reveals account existence), contradicting
  `gip_enumeration_protection_error_collapse.md`. Doesn't invalidate the mapper; verify before leaning on that memory.
- `client.ts` fires `onUnauthorized("no-session")` when `getToken()` is null while `_layout.tsx:60` renders `<Slot/>` without gating on `loading` — pre-existing, unproven, but the new notice makes a misfire show confident wrong copy.
- `support.tsx` fix has no test; spec's mapper table needs the login-validation codes.
- `extra.eas.projectId` is still the literal placeholder `'your-eas-project-id'` — blocks `eas build`.
- Marketing/Settings/Stores-mgmt pages: the mobile API has **no endpoints** for them; needs marketplace-api work first.
