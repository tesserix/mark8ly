# `mcp-catalog` Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Get `mcp-catalog` — merged, tested and sitting on `main` since #663 — actually running in the `mark8ly` namespace and answering a `tools/list`.

**Architecture:** The binary already exists. This plan builds the image in CI, provisions its credential, deploys it via a Helm chart and an ArgoCD Application, registers it in the agent registry, and only then wires it into Kargo's promotion chain.

**Tech Stack:** GitHub Actions (mark8ly), Helm + ArgoCD + External Secrets + KEDA (tesserix-k8s), the agentic registry (devai), GCP Secret Manager.

**Spec:** `docs/superpowers/specs/2026-09-04-mark8ly-mcp-connectors-design.md` — this is the deployment half of its step 2. Steps 3–5 (australis ADR amendment, support-tool migration, platform-read) remain out of scope.

**Repos touched:** `mark8ly`, `tesserix-k8s`, `devai`. Three separate PRs, in the order below.

## Global Constraints

- **The Kargo warehouse is the sharp edge.** `kargo-mark8ly` has ONE `Warehouse` named `services` subscribed to exactly **7** images, and Freight forms only when **all** subscribed images share a tag. Adding an 8th subscription before that image is publishing will **stop Freight forming for every other mark8ly service** — admin, auth-bff, marketplace-api, onboarding, otto, platform-api, storefront all stop deploying. This is why the warehouse change is LAST and gated on evidence.
- **An unreferenced ArgoCD Application manifest fails CI** (`scripts/validate-argocd-apps.py` in tesserix-k8s). A new Application must be wired into the app-of-apps in the same PR that adds it.
- **Chart changes need a `Chart.yaml` version bump** — `ct lint` fails an unbumped chart, and a local `helm lint` will not catch it.
- tesserix-k8s requires a **review approval** before merge. Do not expect to self-merge there.
- The connector gets **its own** key, not the shared gateway's. australis invariant 8: transport identity and data identity must name the same unit.
- No secret value in any manifest, commit, PR body, or log line. Secrets are referenced by GSM key name only.
- Every image reference is pulled through the in-region Artifact Registry mirror (`asia-south1-docker.pkg.dev/tesseracthub-480811/ghcr-remote/tesserix/...`), never `ghcr.io` directly — see `docs/artifact-registry-mirror.md`.

## What already exists — verified 2026-09-05, do not re-derive

| Thing | State |
| --- | --- |
| `services/mcp` on mark8ly `main` | builds, vets, passes `-race`; `Go (mcp)` CI job green |
| `services/mcp/Dockerfile` | builds locally; same base digests and non-root handling as otto |
| Dependabot docker entry for `/services/mcp` | added in #670 |
| `mark8ly-mcp` (the OLD shared gateway) | still running, on the `slm-support-platform` image. **Leave it alone** — retiring it is spec step 4 |
| `STOREFRONT_KEY` | k8s secret `mark8ly-marketplace-api-storefront-key`, from GSM `prod-mark8ly-marketplace-api-storefront-key` |
| Kargo prod stage | currently **Unhealthy** — ArgoCD app `mark8ly-marketplace-api-admin` is Degraded. **Pre-existing and unrelated.** Do not try to fix it here, and do not treat it as caused by this work |

---

### Task 1 (mark8ly): build and publish the image

**Files:**
- Modify: `.github/ci/container-images.json`

**Interfaces:**
- Produces: a published image `.../ghcr-remote/tesserix/mark8ly-mcp-catalog:main-<sha7>` on every push to `main`.

- [ ] **Step 1: Read the existing entries and match them**

Read `.github/ci/container-images.json`. The Go services (`mark8ly-platform-api`, `mark8ly-otto`) are the right model: `context` is the service directory, `dockerfile` is that directory's Dockerfile, `requires_application_build_secret` is `false`, and `smoke_path` is a health endpoint.

- [ ] **Step 2: Add the entry**

```json
{
  "name": "mark8ly-mcp-catalog",
  "context": "services/mcp",
  "dockerfile": "services/mcp/Dockerfile",
  "target": "server",
  "source_root": "services/mcp",
  "build_args": "",
  "requires_application_build_secret": false,
  "trivy_ignore_file": ".trivyignore",
  "smoke_port": 0,
  "smoke_path": "/healthz"
}
```

**`smoke_port` MUST be 0, and this plan originally got it wrong.** Every Go service in this file uses `0` — `platform-api`, `auth-bff`, `marketplace-api`, `otto` — because each hard-fails at boot without secrets CI does not supply, so a live smoke check can never connect. `mcp-catalog` is in exactly that category: `config.Load()` refuses to start without `STOREFRONT_BASE_URL`, `STOREFRONT_KEY` and `MCP_AUTH_KEY`, and CI passes only registry tokens and `NEXT_PUBLIC_*` build args. Only the Next.js apps, whose config arrives as build-time args, carry a live smoke port. `smoke_path` stays `/healthz` because that is the endpoint this service genuinely serves; the siblings' `/health` would be untrue here, and with the port at 0 the path is unused anyway.

**`target` is `server`, not `runtime`** — `services/mcp/Dockerfile:17` reads `AS server`. An earlier draft of this plan said `runtime`; it was wrong, and reading the file is what caught it. Confirm `smoke_port` matches the config default (8765) and that `/healthz` is the path `main.go` actually serves.

- [ ] **Step 3: Run the contract test**

Run: `python3 .github/scripts/test-ci-contract.py`
Expected: PASS. `test_image_matrix_covers_every_deployable` asserts an exact closed set, so it will fail if the set it expects was not updated too — if it fails, read what it wants rather than deleting the assertion.

- [ ] **Step 4: Build the image locally exactly as CI will**

```bash
cd services/mcp && docker build --target runtime -t mcp-catalog:ci-check .
docker run --rm --entrypoint /bin/sh mcp-catalog:ci-check -c 'echo ok' 2>/dev/null || echo "(distroless — no shell, expected)"
```
A distroless image has no shell; that is fine and expected. The build succeeding is the assertion.

- [ ] **Step 5: Commit and open the PR**

```bash
git commit -am "ci(mcp-catalog): build and publish the connector image"
```

- [ ] **Step 6: THE GATE — confirm the image actually published**

After the PR merges, wait for the `main` CI run and confirm the image exists:

```bash
gh api "orgs/tesserix/packages/container/mark8ly-mcp-catalog/versions" \
  --jq '.[]|"\(.name[0:19]) tags=\(.metadata.container.tags|join(","))"' | head -3
```

**Do not start Task 4 until this returns a `main-<sha7>` tag.** Everything about the Kargo ordering depends on this being true, not assumed.

---

### Task 2 (operator action — NOT an agent task): mint the connector's key

**This task cannot be done by an agent and must not be attempted by one.** It mints a credential.

- [ ] **Step 1: Create the GSM secret**

Create a new secret in GCP project `tesseracthub-480811` named **`prod-mark8ly-mcp-catalog-key`**, containing a freshly generated high-entropy key. It must be a NEW value — do not reuse `prod-support-platform-mark8ly-mcp-key`, which belongs to the shared gateway. Per australis invariant 8, this connector owns its own credential so it can be revoked without touching another product.

- [ ] **Step 1b: Know that this key has TWO readers, not one**

The connector reads it to verify inbound calls. **AgentGateway also reads it, to inject on the way out.** Kora's architecture doc (`kora/docs/architecture/agentic-ai-end-to-end.md`, verified 2026-09-04) establishes the sanctioned path: the agent never holds the MCP key at all. AgentGateway verifies a short-lived Zitadel JWT in strict mode — signature, issuer, audience, expiry and an `agentgateway.mcp` role — then injects `X-MCP-Key` itself after selecting a reviewed route.

That injection reads from the Secret `product-mcp-upstream-keys` in namespace `agentgateway-system`, built by `external-secrets/prod/agentgateway-system/externalsecret.yaml`, which already carries `HOMECHEF_MCP_KEY`, `MARK8LY_MCP_KEY`, `PLATFORM_MCP_KEY` and `STOCKPILOT_MCP_KEY`. It is also exactly the `credentialRef.secretName` devai's registry seeds point at.

So the new GSM secret must be wired into **both** places (Task 3 for the connector, Task 3b for the gateway), or the connector will reject everything the gateway sends.

- [ ] **Step 2: Record who holds it**

Whoever will call the connector (the engine, or an operator testing it) needs this value. Record where it is held. Do not paste it into an issue, a PR, or a commit.

- [ ] **Step 3: Confirm it is readable by the cluster**

The `gcp-secret-store` ClusterSecretStore must be able to read it. If other `prod-mark8ly-*` secrets sync, this one will too; confirm rather than assume by checking the ExternalSecret goes `SecretSynced` in Task 3's verification step.

---

### Task 3 (tesserix-k8s): the chart and the Application

**Files:**
- Create: `charts/apps/mark8ly-mcp-catalog/{Chart.yaml,values.yaml}` and `templates/{deployment,service,serviceaccount,externalsecret,keda,pdb,_helpers.tpl}.yaml`
- Create: `argocd/prod/apps/mark8ly/mcp-catalog.yaml`
- Modify: whatever wires `argocd/prod/apps/mark8ly/` into the app-of-apps

**Interfaces:**
- Consumes: the image from Task 1, the GSM secret from Task 2.
- Produces: `mark8ly-mcp-catalog.mark8ly.svc.cluster.local:8765` serving MCP.

- [ ] **Step 1: Copy the closest sibling and read it properly**

`charts/apps/mark8ly-otto/` is the right model — a single-replica Go service in the same namespace with KEDA, a PDB, a ServiceAccount and an AuthorizationPolicy. Copy its structure. **Read every template before adapting it**; do not rename otto's values blindly.

- [ ] **Step 2: values.yaml**

```yaml
namespace: mark8ly
name: mark8ly-mcp-catalog
image:
  repository: asia-south1-docker.pkg.dev/tesseracthub-480811/ghcr-remote/tesserix/mark8ly-mcp-catalog
  tag: "main-REPLACE"   # a REAL published tag from Task 1 step 6; Kargo rewrites it later
  pullPolicy: IfNotPresent
imagePullSecrets:
  - name: ghcr-secret
service:
  type: ClusterIP
  port: 8765
replicaCount: 1
```

**`tag` must be a real, published tag** — not `main`, not `latest`. Take it from Task 1 step 6's output. The spec forbids resolving a moving tag (D8), and the estate has just spent a day on pruned digests.

- [ ] **Step 3: Deployment env — the config the binary actually reads**

Read `services/mcp/internal/config/config.go` in mark8ly and wire exactly what `Load()` requires. As of this writing that is `STOREFRONT_BASE_URL`, `STOREFRONT_KEY`, `MCP_AUTH_KEY`, and optionally `PORT` and `UPSTREAM_TIMEOUT` — **but read the file, because it refuses to start if a required variable is missing, and a stale plan is not a reason for a CrashLoopBackOff.**

`STOREFRONT_BASE_URL` is `http://mark8ly-marketplace-api-storefront.mark8ly.svc.cluster.local:8080` — no path component, and no trailing slash. `STOREFRONT_KEY` and `MCP_AUTH_KEY` come from secret refs, never literals.

Probes: `/healthz` for liveness, `/readyz` for readiness, on port 8765.

- [ ] **Step 4: ExternalSecret**

Two keys, from two different GSM secrets:

| env | GSM key |
| --- | --- |
| `STOREFRONT_KEY` | `prod-mark8ly-marketplace-api-storefront-key` |
| `MCP_AUTH_KEY` | `prod-mark8ly-mcp-catalog-key` (Task 2) |

Follow `charts/apps/console/templates/externalsecret.yaml` for shape: `ClusterSecretStore` `gcp-secret-store`, `refreshInterval: 1h`, `creationPolicy: Owner`.

**Name the resulting k8s Secret `mark8ly-mcp-catalog-secrets`** — step 9's verification and the Deployment's `secretKeyRef`s both use that name, so picking a different one silently breaks both.

- [ ] **Step 5: Bump the chart version**

`Chart.yaml` starts at `version: 0.1.0`. A new chart needs no bump, but confirm `ct lint` is satisfied — an unbumped or malformed `Chart.yaml` fails CI in a way `helm lint` does not reproduce locally.

- [ ] **Step 6: The ArgoCD Application**

Copy `argocd/prod/apps/mark8ly/otto.yaml`. Keep:
- the `kargo.akuity.io/authorized-stage: kargo-mark8ly:prod` annotation
- the `ignoreDifferences` for `/status/terminatingReplicas`
- automated sync with prune and selfHeal

**Then wire it into the app-of-apps.** An unreferenced Application manifest fails `scripts/validate-argocd-apps.py`. Find how the sibling Applications in `argocd/prod/apps/mark8ly/` are enumerated and add this one the same way.

- [ ] **Step 7: Validate before pushing**

```bash
helm lint charts/apps/mark8ly-mcp-catalog
helm template charts/apps/mark8ly-mcp-catalog | head -60
python3 scripts/validate-argocd-apps.py
```
`helm template` must render with no missing values and no secret literals. Grep the output for the string `KEY` and confirm every hit is a `secretKeyRef`, not a value.

- [ ] **Step 8: Commit, PR, and wait for approval**

```bash
git commit -m "feat(mark8ly): deploy the mcp-catalog connector"
```
tesserix-k8s requires a review approval. Do not self-merge.

- [ ] **Step 9: Verify it is actually running**

After ArgoCD syncs:

```bash
kubectl get externalsecret -n mark8ly mark8ly-mcp-catalog-secrets   # must be SecretSynced
kubectl get deploy -n mark8ly mark8ly-mcp-catalog                   # must be 1/1
kubectl logs -n mark8ly deploy/mark8ly-mcp-catalog | tail -20       # must contain NO secret
```

Then call it. **Three headers are mandatory** — without them you get a 400 that looks like auth and is neither:

```bash
KEY=$(kubectl get secret -n mark8ly mark8ly-mcp-catalog-secrets -o jsonpath='{.data.MCP_AUTH_KEY}' | base64 -d)
kubectl run mcpq-$RANDOM -n mark8ly --rm -i --restart=Never --image=curlimages/curl:8.10.1 --quiet --env="K=$KEY" -- sh -c '
curl -s -m 20 -X POST http://mark8ly-mcp-catalog:8765/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "MCP-Protocol-Version: 2026-07-28" \
  -H "Mcp-Method: tools/list" \
  -H "X-MCP-Key: $K" \
  -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}"'
```

**Expected: the five tool names.** `list_store_products`, `get_store_product`, `list_store_categories`, `list_products_by_category`, `get_store_branding`. Anything else — and especially an empty tool list — means stop and diagnose, not proceed. An empty list is exactly the shape of #412.

Then call one tool for real against a live store slug and confirm a projected result comes back with no `null`.

---

### Task 3b (tesserix-k8s): let AgentGateway hold the key

**Files:**
- Modify: `external-secrets/prod/agentgateway-system/externalsecret.yaml`

- [ ] **Step 1: Add the connector's key to `product-mcp-upstream-keys`**

Add a `secretKey` for the catalog connector alongside the existing four, sourced from the GSM secret minted in Task 2. Match the naming of its siblings — they read `<PRODUCT>_MCP_KEY` — and pick a name that distinguishes this connector from the existing `MARK8LY_MCP_KEY`, which belongs to the shared gateway that is NOT being retired here.

- [ ] **Step 2: Verify it syncs**

```bash
kubectl get externalsecret -n agentgateway-system product-mcp-upstream-keys   # SecretSynced
kubectl get secret -n agentgateway-system product-mcp-upstream-keys -o jsonpath='{.data}' | python3 -c "import json,sys; print(sorted(json.load(sys.stdin).keys()))"
```
Expect the new key name in that list. **Print the key NAMES only, never the values.**

---

### Task 3c (tesserix-k8s): admit the gateway at the mesh boundary

**Files:**
- Create/modify: the connector chart's `authorization-policy.yaml`, and whatever enrols it in the ambient mesh

- [ ] **Step 1: Copy how kora-mcp is admitted**

Kora's doc describes two boundaries: the waypoint authorizes the original `agentgateway-mcp` or `support-platform-slm-router` caller at the Service boundary, and the workload boundary admits only those direct callers or the waypoint's forwarding identity. Istio ambient mTLS authenticates every hop by ServiceAccount SPIFFE identity.

**Read how `kora-mcp` actually does this before writing anything** — `charts/apps/kora-mcp/` if it exists, plus `charts/thirdparty/istio-config/templates/network-policies.yaml`, which references `kora-mcp`. Note also that tesserix-k8s recently landed "fix(mcp): enroll AgentGateway in ambient mesh (#956)", so the enrolment mechanism is current and worth reading rather than inferring.

- [ ] **Step 2: Admit only what must be admitted**

The connector should accept calls from the AgentGateway identity, and nothing else. It must NOT be open to the namespace at large. Confirm by reading the rendered policy, not by trusting the values file.

---

### Task 4 (tesserix-k8s): add it to the Kargo warehouse — LAST, and only on evidence

**Do not start this task until Task 1 step 6 has confirmed a published `main-<sha7>` tag, and Task 3 step 9 has confirmed the connector answers a `tools/list`.**

**Files:**
- Modify: the mark8ly Kargo project values that produce the `services` Warehouse

**Interfaces:**
- Produces: Kargo promoting `mark8ly-mcp-catalog` alongside the other seven.

- [ ] **Step 1: Understand what you are changing**

There is ONE Warehouse, `services`, in namespace `kargo-mark8ly`, subscribed to 7 images. Freight forms only when **all** subscribed images share a tag. Adding an 8th means no Freight forms until `mark8ly-mcp-catalog` also publishes that tag.

Because the image builds on every push to `main` from Task 1 onward, this is safe **once that is true and only then**. If it is not true, every mark8ly service stops deploying, and the symptom (Freight simply stops appearing) points nowhere near this change.

- [ ] **Step 2: Record the pre-change state**

```bash
kubectl get freight -n kargo-mark8ly --sort-by=.metadata.creationTimestamp | tail -3
kubectl get warehouse services -n kargo-mark8ly -o jsonpath='{.spec.subscriptions[*].image.repoURL}' | tr ' ' '\n'
```
Write both into the PR. You need to know what "normal" looked like to tell whether you broke it.

- [ ] **Step 3: Add the subscription**

Add `mark8ly-mcp-catalog` to the service list that generates the Warehouse, matching the existing entries.

- [ ] **Step 4: Commit and PR**

```bash
git commit -m "feat(kargo): promote mcp-catalog alongside the other mark8ly services"
```

- [ ] **Step 5: THE GATE — confirm Freight still forms**

Kargo polls every 5 minutes. After the PR merges and ArgoCD syncs:

```bash
kubectl get warehouse services -n kargo-mark8ly -o jsonpath='{.spec.subscriptions[*].image.repoURL}' | tr ' ' '\n' | wc -l   # expect 8
kubectl get freight -n kargo-mark8ly --sort-by=.metadata.creationTimestamp | tail -3
```

**A new Freight must appear within ~15 minutes of the next push to `main`.** If none does, the 8th subscription is not resolving and every mark8ly deploy is now blocked — revert this commit immediately rather than debugging forward. That is the whole reason this task is last and separate.

---

### Task 5 (devai): register the connector

**Files:**
- Create: `architecture/registry-seeds/mcp-servers/mark8ly-mcp-catalog.yaml`

- [ ] **Step 1: Copy the sibling and adapt it**

`architecture/registry-seeds/mcp-servers/mark8ly-mcp.yaml` is the model. Keep `apiVersion: registry.agentic.dev/v1alpha1`, `kind: MCPServer`, the `mcp.tesserix.app/tenant: mark8ly` label, `protocolVersion: '2026-07-28'`, and the `credentialRef` shape.

Changes from the sibling:
- `name` / `displayName`: `mark8ly-mcp-catalog` / "Mark8ly Catalog MCP"
- `endpoint` and `remotes[0].url`: `http://mark8ly-mcp-catalog.mark8ly.svc.cluster.local:8765/mcp`
- `credentialRef.key`: the new key from Task 2, header `X-MCP-Key`
- `tools`: the five catalog tools, **in the same order `Registry.Names()` returns them** — alphabetical
- `description`: say plainly that these are read-only catalog reads over a store's public slug, and that nothing here can write

- [ ] **Step 2: Cross-check against what is actually served**

The tool list in the seed and the tool list the server returns must match. mark8ly's `cmd/mcp-catalog/declared_test.go` pins the server side; this seed is the other side of that comparison. If they disagree, one of them is wrong — find out which before committing.

- [ ] **Step 3: Commit and PR**

```bash
git commit -m "feat(registry): register the mark8ly catalog connector"
```

Note the seeds are a source of record; **zero `MCPServer` CRs exist in the cluster** and devai's catalog seeds declare a different apiVersion group than the installed CRD. Applying seeds is a separate, pre-existing piece of work — do not take it on here.

---

## Out of scope

- **Retiring the old `mark8ly-mcp` gateway.** Spec step 4, after the support tools migrate. Both run side by side until then; that is the temporary two-pod cost the spec already accounted for.
- **The Degraded `mark8ly-marketplace-api-admin` Application.** Pre-existing, unrelated, and the reason the Kargo prod stage reads Unhealthy today.
- **Applying registry seeds as cluster CRs**, and the `registry.solo.io` vs `kagent.dev` apiVersion mismatch.
- **AgentGateway routing.** The registry record names the in-cluster Service URL. Whether callers go through AgentGateway (ADR-0001 D4) is open question 1 in the spec and is not settled by deploying this.
- **A `/readyz` that probes the upstream.** It returns a static 200, matching otto. Worth revisiting when something gates on it.

## Open questions

1. **Who calls this first?** Deploying it proves it runs; it does not prove anything consumes it. Identify the first caller before Task 4, because a connector nothing calls is not obviously worth adding to the promotion chain.
2. ~~Does the engine reach it directly or via AgentGateway?~~ **ANSWERED — via AgentGateway.** Kora's architecture doc settles it: the agent presents a Zitadel JWT to AgentGateway, which authenticates it and injects `X-MCP-Key` from `product-mcp-upstream-keys`. That drove Tasks 3b and 3c into this plan; they were missing from the first draft.
3. **How is an MCP server registered as an AgentGateway ROUTE?** Still open, and the one piece of the path not yet established. `charts/thirdparty/agentgateway/values.yaml`'s `backends` block is for LLM providers (anthropic/openai/groq), not MCP servers, so routing lives somewhere else. Find how `kora-mcp` is reached before assuming a deployed Service is callable — a connector the gateway has no route to is running but unreachable, which is #412's shape again.
