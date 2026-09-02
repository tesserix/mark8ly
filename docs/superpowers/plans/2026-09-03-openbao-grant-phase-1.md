# OpenBao Grant for mark8ly (Phase 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Grant mark8ly's two marketplace-api ServiceAccounts authenticated access to OpenBao, and prove the grant works with a live round-trip — before any credential code depends on it.

**Architecture:** Three separate grants must all line up: an L3 NetworkPolicy allowance (`openbao-namespace` chart), an Istio principal allowance for mesh-enrolled callers (same chart), and an OpenBao policy + Kubernetes auth role (`thirdparty/openbao` chart). A `scope-probe` Job then logs in with a real ServiceAccount token and round-trips a dummy secret. No mark8ly application code changes in this phase.

**Tech Stack:** Helm, ArgoCD, OpenBao KV v2, Kubernetes auth, NetworkPolicy, Istio AuthorizationPolicy.

**Spec:** `docs/superpowers/specs/2026-09-03-openbao-carrier-secrets-design.md`

## Global Constraints

- Repo for every change in this phase: `tesserix-k8s`. No `mark8ly` changes.
- **Every chart you edit needs its `Chart.yaml` `version` bumped.** `ct lint` fails an unbumped chart; local `helm lint` does not catch it.
- Branch from `origin/main` and never rewrite another branch — this repo has concurrent sessions.
- PRs in `tesserix-k8s` require a review approval; they cannot self-merge.
- Policy names must NOT start with `app-`. The secret-service console owns that prefix at runtime and the bootstrap Job never reconciles it.
- OpenBao path convention is namespace-prefixed: `kv/data/mark8ly/marketplace-api/...`. No `<env>` segment.
- The two ServiceAccounts are `mark8ly-marketplace-api-admin` and `mark8ly-marketplace-api-storefront`, both in namespace `mark8ly`.

---

## File Structure

| File | Responsibility |
| --- | --- |
| `charts/apps/openbao-namespace/values.yaml` | L3 NetworkPolicy sources + Istio principals. Adds mark8ly pods. |
| `charts/apps/openbao-namespace/Chart.yaml` | Version bump. |
| `charts/thirdparty/openbao/values.yaml` | OpenBao policies + Kubernetes auth roles. Adds the two mark8ly grants. |
| `charts/thirdparty/openbao/Chart.yaml` | Version bump. |
| `argocd/prod/projects/security.yaml` | ArgoCD destination for the namespace, if absent. |

---

### Task 1: Verify what is actually missing before changing anything

The grant has three independent halves and this repo has burned two days on assuming one of them. Establish the real starting state first.

**Files:** none (read-only investigation).

**Interfaces:**
- Consumes: nothing.
- Produces: a written finding recorded in the PR description — whether `security.yaml` already lists the `mark8ly` destination, and the exact current `allowedPodSources` / `policies` / `kubernetesRoles` entries.

- [ ] **Step 1: Confirm the ArgoCD destination**

```bash
cd tesserix-k8s
grep -n "mark8ly" argocd/prod/projects/security.yaml || echo "ABSENT — Task 4 is required"
```

Expected: either a `mark8ly` destination line, or `ABSENT`. The openbao values file states a new namespace also needs a `destinations` entry here or ArgoCD refuses to render its store.

- [ ] **Step 2: Confirm no mark8ly grant exists yet**

```bash
grep -n "mark8ly" charts/thirdparty/openbao/values.yaml || echo "no openbao grant"
grep -n "mark8ly" charts/apps/openbao-namespace/values.yaml || echo "no networkpolicy grant"
```

Expected: both absent. If either is present, STOP and re-read — someone else is mid-flight.

- [ ] **Step 3: Record the findings**

Write the two results into the PR description you will open in Task 5. No commit.

---

### Task 2: Add the OpenBao policies and Kubernetes auth roles

**Files:**
- Modify: `charts/thirdparty/openbao/values.yaml` (`bootstrap.policies`, `bootstrap.kubernetesRoles`)
- Modify: `charts/thirdparty/openbao/Chart.yaml` (version bump)

**Interfaces:**
- Consumes: nothing.
- Produces: OpenBao policies `mark8ly-marketplace-api-admin-carrier-secrets` and `mark8ly-marketplace-api-storefront-carrier-secrets`; Kubernetes auth roles `mark8ly-marketplace-api-admin` and `mark8ly-marketplace-api-storefront`. Task 3 and Task 5 both reference these role names verbatim.

- [ ] **Step 1: Add the two policies**

In `charts/thirdparty/openbao/values.yaml`, inside `bootstrap.policies`, after the `read-platform` entry:

```yaml
    # mark8ly per-tenant carrier credentials (mark8ly#319). NOT a
    # `namespaceWhitelist` entry: that generates a read-only policy at
    # kv/data/<ns>/<app>/*, and this app must CREATE and UPDATE secrets a
    # merchant saves, and DELETE metadata when a credential is removed.
    #
    # Two policies, least privilege. Every Put and Destroy in marketplace-api
    # is admin-side (handlers/admin/settings.go, internal/domain/service.go);
    # storefront handlers only read, so storefront gets no write capability.
    - name: mark8ly-marketplace-api-admin-carrier-secrets
      hcl: |
        path "kv/data/mark8ly/marketplace-api/tenants/*"     { capabilities = ["create", "read", "update"] }
        path "kv/metadata/mark8ly/marketplace-api/tenants/*" { capabilities = ["read", "list", "delete"] }
    - name: mark8ly-marketplace-api-storefront-carrier-secrets
      hcl: |
        path "kv/data/mark8ly/marketplace-api/tenants/*"     { capabilities = ["read"] }
        path "kv/metadata/mark8ly/marketplace-api/tenants/*" { capabilities = ["read", "list"] }
```

- [ ] **Step 2: Add the two Kubernetes auth roles**

In the same file, inside `bootstrap.kubernetesRoles`, after the `read-platform` entry:

```yaml
    - name: mark8ly-marketplace-api-admin
      serviceAccounts:
        - mark8ly-marketplace-api-admin
      namespaces:
        - mark8ly
      policies:
        - mark8ly-marketplace-api-admin-carrier-secrets
      ttl: 1h
    - name: mark8ly-marketplace-api-storefront
      serviceAccounts:
        - mark8ly-marketplace-api-storefront
      namespaces:
        - mark8ly
      policies:
        - mark8ly-marketplace-api-storefront-carrier-secrets
      ttl: 1h
```

- [ ] **Step 3: Bump the chart version**

In `charts/thirdparty/openbao/Chart.yaml`, increment the `version:` patch number by one.

- [ ] **Step 4: Verify the bootstrap ConfigMap renders both policies and both roles**

```bash
helm template t charts/thirdparty/openbao \
  | grep -E "policy-mark8ly-|role-mark8ly-"
```

Expected: four lines — `policy-mark8ly-marketplace-api-admin-carrier-secrets.hcl`, `policy-mark8ly-marketplace-api-storefront-carrier-secrets.hcl`, `role-mark8ly-marketplace-api-admin.json`, `role-mark8ly-marketplace-api-storefront.json`.

If the role JSON is missing, the entry landed under `policies` instead of `kubernetesRoles`.

- [ ] **Step 5: Verify the rendered role binds the right ServiceAccount**

```bash
helm template t charts/thirdparty/openbao \
  | grep -A 12 "role-mark8ly-marketplace-api-admin.json"
```

Expected: `"bound_service_account_names": ["mark8ly-marketplace-api-admin"]`, `"bound_service_account_namespaces": ["mark8ly"]`, `"token_policies": ["mark8ly-marketplace-api-admin-carrier-secrets"]`.

- [ ] **Step 6: Lint**

```bash
helm lint charts/thirdparty/openbao
```

Expected: `0 chart(s) failed`.

- [ ] **Step 7: Commit**

```bash
git add charts/thirdparty/openbao/values.yaml charts/thirdparty/openbao/Chart.yaml
git commit -m "feat(openbao): grant mark8ly marketplace-api access to per-tenant carrier secrets"
```

---

### Task 3: Allow mark8ly pods through the NetworkPolicy and Istio policy

The OpenBao role decides *what a token may do*. It does not decide *whether the packet arrives*. Both halves are required, and a missing L3 allowance fails **silently** — the connection is dropped, not refused, so the client hangs until its deadline. This is exactly how mark8ly#587 failed twice.

**Files:**
- Modify: `charts/apps/openbao-namespace/values.yaml` (`allowedPodSources`, `allowedPrincipals`)
- Modify: `charts/apps/openbao-namespace/Chart.yaml` (version bump)

**Interfaces:**
- Consumes: the role names from Task 2 (referenced only in comments).
- Produces: L3 + Istio reachability for pods labelled `mark8ly-marketplace-api-admin` and `mark8ly-marketplace-api-storefront` in namespace `mark8ly`.

- [ ] **Step 1: Add the pod sources**

In `charts/apps/openbao-namespace/values.yaml`, inside `allowedPodSources`:

```yaml
  # mark8ly marketplace-api (mark8ly#319). Two deployments, two pod labels.
  #
  # NOTE for whoever adds the CronJobs in phase 4: a CronJob's pods carry a
  # DIFFERENT app.kubernetes.io/name (e.g.
  # mark8ly-marketplace-api-admin-refund-sweep) even though they run under the
  # admin ServiceAccount. The OpenBao role already covers them; this list does
  # not. That mismatch is what broke mark8ly#587 — same identity, different
  # label, silent timeout.
  - namespace: mark8ly
    podLabels:
      app.kubernetes.io/name: mark8ly-marketplace-api-admin
  - namespace: mark8ly
    podLabels:
      app.kubernetes.io/name: mark8ly-marketplace-api-storefront
```

- [ ] **Step 2: Add the Istio principals**

marketplace-api pods run with an Istio sidecar, so they have a SPIFFE identity and the AuthorizationPolicy applies on top of the NetworkPolicy. In the same file, inside `allowedPrincipals`:

```yaml
  - mark8ly/mark8ly-marketplace-api-admin
  - mark8ly/mark8ly-marketplace-api-storefront
```

- [ ] **Step 3: Bump the chart version**

In `charts/apps/openbao-namespace/Chart.yaml`, increment the `version:` patch number by one.

- [ ] **Step 4: Verify both the NetworkPolicy and the AuthorizationPolicy render**

```bash
helm template t charts/apps/openbao-namespace | grep -B 3 -A 3 "mark8ly-marketplace-api"
```

Expected: the two pod labels appear under a `podSelector` in the NetworkPolicy, AND the two `mark8ly/...` principals appear in the AuthorizationPolicy. If only one of the two shows up, the grant is half-made and Task 5 will fail.

- [ ] **Step 5: Lint**

```bash
helm lint charts/apps/openbao-namespace
```

Expected: `0 chart(s) failed`.

- [ ] **Step 6: Commit**

```bash
git add charts/apps/openbao-namespace/values.yaml charts/apps/openbao-namespace/Chart.yaml
git commit -m "feat(openbao): allow mark8ly marketplace-api pods to reach the OpenBao API"
```

---

### Task 4: Add the ArgoCD destination (only if Task 1 found it absent)

If Task 1 Step 1 printed a `mark8ly` line, SKIP this task entirely and say so in the PR.

**Files:**
- Modify: `argocd/prod/projects/security.yaml`

**Interfaces:**
- Consumes: the Task 1 finding.
- Produces: permission for the security AppProject to render resources into the `mark8ly` namespace.

- [ ] **Step 1: Read the existing destinations block**

```bash
sed -n '/destinations:/,/^  [a-z]/p' argocd/prod/projects/security.yaml
```

- [ ] **Step 2: Add the destination, matching the surrounding entries exactly**

```yaml
    - namespace: mark8ly
      server: https://kubernetes.default.svc
```

Match the indentation and `server:` value of the neighbouring entries rather than copying the above verbatim — if the file uses a named cluster instead of `kubernetes.default.svc`, use that.

- [ ] **Step 3: Verify it parses**

```bash
python3 -c "import yaml,sys; d=yaml.safe_load(open('argocd/prod/projects/security.yaml')); print([x for x in d['spec']['destinations']])"
```

Expected: a list including `{'namespace': 'mark8ly', ...}`.

- [ ] **Step 4: Commit**

```bash
git add argocd/prod/projects/security.yaml
git commit -m "feat(argocd): allow the security project to target the mark8ly namespace"
```

---

### Task 5: Prove the grant with a live scope-probe

A rendered manifest is not a working grant. This is the task that actually closes phase 1, and it runs **after** the PR from Tasks 2–4 is merged and synced.

**Files:** none committed — the probe is a throwaway Job applied by hand and deleted.

**Interfaces:**
- Consumes: role `mark8ly-marketplace-api-admin` from Task 2; L3 + Istio allowance from Task 3.
- Produces: evidence that a real ServiceAccount token can log in and round-trip a secret. Phase 2 depends on nothing else from this phase.

- [ ] **Step 1: Confirm the bootstrap Job has re-run and created the role**

The policies and roles are written by the `openbao-bootstrap` Job, which runs on chart sync. Confirm it completed AFTER your merge:

```bash
kubectl -n openbao get jobs -l app.kubernetes.io/component=bootstrap \
  --sort-by=.metadata.creationTimestamp
kubectl -n openbao logs job/<newest-bootstrap-job> | grep -i "mark8ly"
```

Expected: lines showing `policy/mark8ly-marketplace-api-admin-carrier-secrets` and `role/mark8ly-marketplace-api-admin`. If the Job has not re-run, the grant is not live no matter what the repo says — trigger a sync and wait.

- [ ] **Step 2: Apply the scope-probe Job**

Runs under the real admin ServiceAccount, so it exercises the true identity. Istio injection is disabled to keep the Job short-lived, which means it is admitted by the NetworkPolicy path rather than the Istio principal — Step 5 covers the mesh path.

```bash
cat <<'YAML' | kubectl apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: mark8ly-bao-scope-probe
  namespace: mark8ly
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 600
  template:
    metadata:
      annotations:
        sidecar.istio.io/inject: "false"
    spec:
      restartPolicy: Never
      serviceAccountName: mark8ly-marketplace-api-admin
      containers:
        - name: probe
          image: curlimages/curl:8.10.1
          command: ["/bin/sh", "-c"]
          args:
            - |
              set -e
              BAO=http://openbao-active.openbao.svc.cluster.local:8200
              JWT=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
              echo "==> login"
              TOKEN=$(curl -sS -X POST "$BAO/v1/auth/kubernetes/login" \
                -d "{\"role\":\"mark8ly-marketplace-api-admin\",\"jwt\":\"$JWT\"}" \
                | sed -n 's/.*"client_token":"\([^"]*\)".*/\1/p')
              test -n "$TOKEN" || { echo "LOGIN FAILED"; exit 1; }
              echo "==> login ok"
              P=kv/data/mark8ly/marketplace-api/tenants/_probe/payment/_probe/api_key
              echo "==> write"
              curl -sS -f -X POST "$BAO/v1/$P" -H "X-Vault-Token: $TOKEN" \
                -d '{"data":{"value":"probe-value"}}' >/dev/null
              echo "==> read"
              curl -sS -f "$BAO/v1/$P" -H "X-Vault-Token: $TOKEN" \
                | grep -q "probe-value" || { echo "READ MISMATCH"; exit 1; }
              echo "==> delete"
              curl -sS -f -X DELETE \
                "$BAO/v1/kv/metadata/mark8ly/marketplace-api/tenants/_probe/payment/_probe/api_key" \
                -H "X-Vault-Token: $TOKEN" >/dev/null
              echo "PROBE OK"
YAML
```

- [ ] **Step 3: Read the result**

```bash
kubectl -n mark8ly wait --for=condition=complete job/mark8ly-bao-scope-probe --timeout=120s \
  || kubectl -n mark8ly describe job/mark8ly-bao-scope-probe
kubectl -n mark8ly logs -l job-name=mark8ly-bao-scope-probe
```

Expected: `==> login ok`, `==> write`, `==> read`, `==> delete`, `PROBE OK`.

Failure reading guide:
- **Job times out with no log output** → L3 denial. Task 3 Step 1 did not land, or the pod label is wrong. Check `kubectl -n mark8ly get pods -l job-name=mark8ly-bao-scope-probe --show-labels`.
- **`LOGIN FAILED`** → the role does not exist or does not bind this ServiceAccount. Re-check Task 2 Step 5 and the bootstrap logs from Step 1.
- **HTTP 403 on write** → the token authenticated but the policy is read-only. The entry likely went in as a `namespaceWhitelist` app instead of an explicit policy.

- [ ] **Step 4: Verify least privilege actually holds**

The storefront role must NOT be able to write. This asserts the security property rather than assuming it.

```bash
# Same Job, but serviceAccountName: mark8ly-marketplace-api-storefront
# and role: mark8ly-marketplace-api-storefront.
# The write step MUST fail with 403.
```

Re-apply the Step 2 Job with `metadata.name: mark8ly-bao-scope-probe-ro`, `serviceAccountName: mark8ly-marketplace-api-storefront`, and the role name changed to `mark8ly-marketplace-api-storefront`.

Expected: `==> login ok` followed by a **failure at the write step** (curl `-f` exits non-zero on 403). A storefront probe that reports `PROBE OK` means the least-privilege split is not in effect — treat that as a blocking defect, not a pass.

- [ ] **Step 5: Verify the mesh path**

Steps 2–4 bypass Istio. Confirm a sidecar-injected pod is also admitted, since that is how the real deployments talk:

```bash
kubectl -n mark8ly exec deploy/mark8ly-marketplace-api-admin -c istio-proxy -- \
  curl -sS -o /dev/null -w '%{http_code}\n' \
  http://openbao-active.openbao.svc.cluster.local:8200/v1/sys/health
```

Expected: `200` (or `429`/`473` for standby, both of which prove reachability). A hang or `000` means the Istio principal from Task 3 Step 2 is missing.

- [ ] **Step 6: Clean up**

```bash
kubectl -n mark8ly delete job mark8ly-bao-scope-probe mark8ly-bao-scope-probe-ro --ignore-not-found
```

- [ ] **Step 7: Record the evidence on the issue**

Post the probe output to mark8ly#319 as a comment. Phase 1 is complete only when that evidence exists — a merged PR is not the deliverable.

---

## Phase 1 Done Criteria

- [ ] Both policies and both roles appear in the `openbao-bootstrap` Job log after a post-merge run
- [ ] Admin probe: login, write, read, delete all succeed
- [ ] Storefront probe: login succeeds, write is **refused with 403**
- [ ] A sidecar-injected pod reaches the OpenBao API
- [ ] Evidence posted to mark8ly#319

Phase 2 (`ChainStore`, `BaoStore`, cache) does not begin until all five are ticked.
