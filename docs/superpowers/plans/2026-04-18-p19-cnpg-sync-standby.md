# P19 — CNPG Sync-Standby Configuration at 100-Merchant Tier Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver RPO=0 / RTO≤2 min for `marketplace-api`'s Postgres by edit-only work on the CNPG `Cluster` CR in `tesserix-k8s`: set `synchronous_commit = on` (not the CNPG default `local`), scale to `instances: 2` (primary + synchronous standby), enable `primaryUpdateStrategy: unsupervised` for auto-failover, and configure `spec.postgresql.synchronous.method: any / number: 1` so exactly one standby acknowledges every transaction. Write the primary-failover runbook, extend the P17 health dashboard with replication-lag panels + alerts, and validate the whole thing end-to-end against the running cluster.

**Architecture:** Entirely YAML + ops. No Go code, no migrations, no application retry logic. The CNPG operator watches the `Cluster` CR, provisions the second instance, streams a base-backup from primary, promotes it to a synchronous standby, and thereafter rejects commits that cannot be acknowledged by the standby (because `synchronous_commit = on` + `synchronous_standby_names = ANY(1, *)`). ArgoCD (auto-sync, self-heal) picks the change up from the tesserix-k8s overlay and rolls it out. marketplace-api's existing connection string points at the CNPG-managed `-rw` Service, so failover is handled by the operator rewriting the endpoint — the pod's GORM pool reconnects on error. Dashboards and the failover runbook complete the operational story.

**Tech Stack:** CloudNativePG (operator + `Cluster` CRD), Kubernetes 1.30+, ArgoCD (ApplicationSet-driven sync), Grafana + Prometheus (scraping CNPG's `/metrics` endpoint), `kubectl`, the `cnpg` kubectl plugin, Postgres 15.

**Spec:** [`docs/superpowers/specs/2026-04-17-subscription-model-design.md`](../specs/2026-04-17-subscription-model-design.md) — §22.2 (RTO/RPO table), §22 (DR context), §24 (CNPG staircase), specifically the **"SA finding: synchronous_commit = on"** callout at end of §24.

**Depends on:** Nothing code-side. P1–P17 can land before or after; this plan must land **before public launch** per §22 ("Sync standby at 100-merchant tier"). P17's health dashboard exists and has a free panel slot; if P17 hasn't shipped, Task 4 is best-effort deferred.

**Related plans:**
- **P17** (CNPG health dashboard) — consumes the two new metric panels introduced here
- **P20** (PgBouncer connection pooling) — arrives together at the 100-merchant tier per §24 but is scoped separately

---

## Scope Check

In scope:
1. ArgoCD overlay edit in tesserix-k8s: find the `Cluster` CR that backs marketplace-api's database; bump `instances` from 1 → 2, set `synchronous_commit = on`, set `primaryUpdateStrategy: unsupervised`, configure `spec.postgresql.synchronous.method: any / number: 1`.
2. Resource block update per §24 staircase (100-merchant tier: 1 CPU / 4 GiB memory / 200 GiB storage). Validate current values, diff, apply.
3. `docs/runbooks/cnpg-primary-failover.md` — auto-failover behaviour, RTO at this tier, reader/writer Service endpoints, manual promotion via `cnpg` CLI, post-failover validation queries.
4. Extend P17's CNPG health dashboard: add `pg_stat_replication.write_lag` and `replay_lag` panels; Prometheus alert rule firing when sync standby lag >5s sustained 1 min.
5. End-to-end verification after ArgoCD sync: `kubectl get cluster` shows 2 instances; `SHOW synchronous_commit` returns `on`; `pg_stat_replication` shows `sync_state = sync` for the standby; a forced primary-pod kill in staging demonstrates marketplace-api resuming within 2 minutes.

Out of scope:
- Application-side failover logic — the existing GORM pool reconnects via CNPG's `*-rw` Service; no code change.
- Region-level DR (§22: 24h+ RTO) — separate project.
- Migration of existing data to the new replica — CNPG provisions the standby via streaming base-backup automatically.
- PgBouncer pool (§24 calls it out at the same tier) — lives in P20.
- Async (non-synchronous) read replicas for the 500–2,000 tier — lives in a future phase.

---

## File Structure

### Modify (in `tesserix-k8s`)

- `tesserix-k8s/apps/marketplace-api/cluster.yaml` — CNPG `Cluster` CR. **Location to verify at Task 1 Step 1** — may instead live at `tesserix-k8s/apps/platform/marketplace-api/cnpg-cluster.yaml` depending on how the repo was laid out in P16. Grep for `kind: Cluster` + `marketplace` under the tesserix-k8s tree; edit exactly one file.
- `tesserix-k8s/apps/marketplace-api/kustomization.yaml` — only if the resource reference needs updating after a rename; usually no change.

### Create (in `mark8ly`)

- `docs/runbooks/cnpg-primary-failover.md` — operator runbook.

### Modify (in `tesserix-infra`)

- `tesserix-infra/grafana/dashboards/subscription-health.json` — add two panels (replication `write_lag`, `replay_lag`) to the existing P17 dashboard. Skip if P17 hasn't yet produced this file; instead write the panel JSON into a dedicated `cnpg-replication.json` file to be merged into P17 later.
- `tesserix-infra/prometheus/rules/cnpg-alerts.yaml` — add `CNPGSyncStandbyLagHigh` rule (may already exist; merge don't overwrite).

### Delete

None.

---

## Task Sequence Overview

| # | Task | Depends on |
|---|---|---|
| 1 | Locate CNPG `Cluster` CR + diff current config against target | — |
| 2 | Edit `Cluster` CR: sync-commit, 2 instances, resources, unsupervised failover | 1 |
| 3 | Apply via ArgoCD + verify rollout (instances, sync_state, sync-commit) | 2 |
| 4 | Dashboard + alert: replication lag panels, `>5s` sustained alert | 3 |
| 5 | Runbook + kill-primary drill + sign-off | 3 |

---

## Reusable patterns

**A. CNPG `Cluster` CR shape** — the synchronous-replication block lives at `spec.postgresql.synchronous` (CNPG ≥1.24) and `spec.postgresql.parameters.synchronous_commit`. `instances` is a top-level scalar. `primaryUpdateStrategy` is top-level. All four edits go into one file; changes are atomic from ArgoCD's perspective.

**B. ArgoCD auto-sync flow** — commit to tesserix-k8s `main` → ApplicationSet reconciles within ~2 minutes → CNPG operator reconciles the `Cluster` resource → creates the second pod, runs base-backup, promotes to synchronous standby. No manual `argocd app sync` needed because auto-sync + self-heal are already on per the repo's ApplicationSet defaults.

**C. Verification queries pattern** — every verification step is a one-liner of the form `kubectl exec -n platform marketplace-db-1 -- psql -U postgres -c "<SQL>"`. Keep them in the runbook so on-call can rerun any of them at 3am.

**D. Rollback pattern** — if the second instance fails to come up, revert the commit (`git revert`). ArgoCD re-applies the single-instance CR and CNPG scales down cleanly. Do **not** `kubectl edit` the live CR — ArgoCD will overwrite.

**E. Kill-primary drill idiom** — `kubectl delete pod -n platform marketplace-db-1 --grace-period=0 --force` simulates primary loss. Watch `kubectl get cluster -n platform marketplace-db -w` and measure the wall-clock from pod-deletion to `phase: Cluster in healthy state`. Target <120s.

---

## Task 1: Locate CNPG Cluster CR + diff current config against target

**Files:**
- Read only: whichever file in `tesserix-k8s` contains the `Cluster` CR for `marketplace-api`.

**Spec references:** §24 (target sizing + sync-commit requirement).

- [ ] **Step 1: Find the file**

```bash
# Run inside a tesserix-k8s checkout.
grep -rln --include='*.yaml' -E '^kind:[[:space:]]+Cluster$' apps/ | xargs grep -l 'marketplace'
```

Expected output: exactly one path. Record it as `$CLUSTER_CR` and use it in all subsequent tasks. If zero or multiple matches come back, stop and ask the operator — the plan assumes P16 shipped a single Cluster CR for marketplace-api.

- [ ] **Step 2: Snapshot the current state**

```bash
kubectl get cluster -n platform marketplace-db -o yaml > /tmp/cluster-before.yaml
cp "$CLUSTER_CR" /tmp/cluster-cr-before.yaml
```

Keep both files around for the rollback / forensic trail. Attach to the PR description.

- [ ] **Step 3: Diff current vs target**

Compare against the target values below. Note every difference — every one becomes an edit in Task 2. The five that MUST be present on exit:

| Field | Current (expected: launch tier) | Target (100-merchant tier) |
|---|---|---|
| `spec.instances` | `1` | `2` |
| `spec.primaryUpdateStrategy` | unset / `supervised` | `unsupervised` |
| `spec.postgresql.parameters.synchronous_commit` | unset (defaults to `local`) | `"on"` |
| `spec.postgresql.synchronous` | unset | `{ method: any, number: 1 }` |
| `spec.resources.requests.cpu` | `500m` | `"1"` |
| `spec.resources.requests.memory` | `2Gi` | `4Gi` |
| `spec.resources.limits.cpu` | `500m` | `"1"` |
| `spec.resources.limits.memory` | `2Gi` | `4Gi` |
| `spec.storage.size` | `50Gi` | `200Gi` |

- [ ] **Step 4: Confirm backup + WAL archiving config unchanged**

`spec.backup.*` must be identical after the edit. Extract and hash:

```bash
yq '.spec.backup' "$CLUSTER_CR" | sha256sum > /tmp/backup-hash-before.txt
```

Hash gets reused at Task 2 Step 4 to prove nothing was accidentally dropped.

- [ ] **Step 5: Note operator version**

```bash
kubectl get deploy -n cnpg-system cnpg-controller-manager -o jsonpath='{.spec.template.spec.containers[0].image}'
```

Record it — `spec.postgresql.synchronous` requires CNPG ≥1.24. If operator is older, **escalate**: operator upgrade is out of scope for this plan and must land first.

- [ ] **Step 6: Commit the snapshots into the plan log (no code change)**

```bash
# In the mark8ly repo, under whatever phase-artifacts location P19 uses.
mkdir -p docs/superpowers/artifacts/p19
cp /tmp/cluster-before.yaml      docs/superpowers/artifacts/p19/cluster-live-before.yaml
cp /tmp/cluster-cr-before.yaml   docs/superpowers/artifacts/p19/cluster-cr-before.yaml
cp /tmp/backup-hash-before.txt   docs/superpowers/artifacts/p19/backup-hash-before.txt
git add docs/superpowers/artifacts/p19
git commit -m "chore(p19): snapshot CNPG cluster pre-edit"
```

---

## Task 2: Edit `Cluster` CR — sync-commit, 2 instances, resources, unsupervised failover

**Files:**
- Modify: `$CLUSTER_CR` (located in Task 1 Step 1).

**Spec references:** §22.2 (2 min RTO / 0 RPO at 100-merchant tier), §24 (sizing + `synchronous_commit = on` requirement).

- [ ] **Step 1: Apply the edits**

The finished `spec:` must look like this (existing fields not shown are preserved as-is):

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: marketplace-db
  namespace: platform
spec:
  instances: 2                            # was: 1 — one primary + one synchronous standby
  primaryUpdateStrategy: unsupervised     # auto-failover on primary pod liveness fail

  postgresql:
    parameters:
      # CRITICAL (§24 SA finding): default is 'local' which does NOT give RPO=0.
      synchronous_commit: "on"
      # keep every other parameter exactly as it was before this edit.
    synchronous:
      method: any                          # ANY(1, *) — any single standby acks
      number: 1                            # require exactly one standby ack per commit

  resources:
    requests:
      cpu:    "1"
      memory: "4Gi"
    limits:
      cpu:    "1"
      memory: "4Gi"

  storage:
    size: 200Gi
    # storageClass: keep whatever it was (typically premium-rwo)

  # spec.backup — MUST remain byte-identical to pre-edit (see Task 1 Step 4 hash)
  # spec.bootstrap — MUST remain unchanged
  # spec.monitoring — unchanged
```

- [ ] **Step 2: Lint**

```bash
kubectl --dry-run=client apply -f "$CLUSTER_CR"
# And if kubeconform is available in the repo's CI tooling:
kubeconform -schema-location default -strict "$CLUSTER_CR"
```

Both must return clean.

- [ ] **Step 3: Confirm backup block preserved**

```bash
yq '.spec.backup' "$CLUSTER_CR" | sha256sum
```

Hash must equal the Task 1 Step 4 hash. If it changed, STOP — you accidentally touched the backup config. Revert the `spec.backup` portion verbatim from `cluster-cr-before.yaml`.

- [ ] **Step 4: Confirm storage resize is expansion, not shrink**

CNPG (and underlying PVC) will reject shrinks. If the current `spec.storage.size` is already >200Gi (unlikely but possible if someone pre-scaled), keep the larger value instead of stepping it down.

- [ ] **Step 5: Commit to tesserix-k8s**

```bash
git -C "$TESSERIX_K8S" add "$CLUSTER_CR"
git -C "$TESSERIX_K8S" commit -m "feat(cnpg): marketplace-db sync standby + 100-merchant tier sizing"
git -C "$TESSERIX_K8S" push origin main
```

Single-line commit message per the project convention. No multi-line body.

---

## Task 3: Apply via ArgoCD + verify rollout

**Files:** none — runtime verification.

**Spec references:** §22.2 (RTO 2 min target), §24 (sync-commit = on must be explicit).

- [ ] **Step 1: Watch the ArgoCD reconcile**

```bash
argocd app wait marketplace-api-db --health --timeout 600
# or, if there's no dedicated DB app, watch the platform ApplicationSet child:
argocd app list | grep marketplace-db
```

Must reach `Synced / Healthy` within 10 min.

- [ ] **Step 2: Verify instance count**

```bash
kubectl get cluster -n platform marketplace-db \
  -o jsonpath='{.status.instances}{"\n"}'
# Expected: 2
kubectl get cluster -n platform marketplace-db \
  -o jsonpath='{.status.readyInstances}{"\n"}'
# Expected: 2
```

Both must be `2`. `readyInstances < 2` means the standby didn't finish base-backup — wait up to 30 min on a freshly provisioned 200Gi volume before escalating.

- [ ] **Step 3: Verify `synchronous_commit = on`**

```bash
kubectl exec -n platform marketplace-db-1 -- \
  psql -U postgres -tAc "SHOW synchronous_commit;"
# Expected: on
```

If output is `local`, the parameter was not propagated. Check `spec.postgresql.parameters` in the live CR with `kubectl get cluster -o yaml` vs the file. Do NOT `kubectl edit` — fix in git and re-sync.

- [ ] **Step 4: Verify standby is synchronous**

```bash
kubectl exec -n platform marketplace-db-1 -- psql -U postgres -tAc "
  SELECT application_name, sync_state, state
  FROM pg_stat_replication;"
# Expected: exactly one row, sync_state = sync, state = streaming
```

Any row with `sync_state = async` or zero rows means the `synchronous.method/number` block didn't take effect.

- [ ] **Step 5: Verify reader + writer Services exist**

```bash
kubectl get svc -n platform | grep marketplace-db
# Expected services:
#   marketplace-db-rw   (writer — primary only)
#   marketplace-db-ro   (read-only — standbys only)
#   marketplace-db-r    (any replica — primary + standbys)
```

marketplace-api connects via `-rw` (unchanged). `-ro` / `-r` get used by read-replica consumers in a later phase; existence confirmation is enough here.

- [ ] **Step 6: Smoke test marketplace-api**

```bash
kubectl rollout status deploy/marketplace-api -n marketplace --timeout=5m
curl -sSf https://<marketplace-api-host>/health | jq .
```

Health endpoint must return 200. Tail logs briefly (`kubectl logs -n marketplace -l app=marketplace-api --tail=50`) and confirm no `pq: connection refused` spam that would indicate pool churn.

- [ ] **Step 7: Commit verification artifacts**

```bash
kubectl get cluster -n platform marketplace-db -o yaml > /tmp/cluster-after.yaml
cp /tmp/cluster-after.yaml docs/superpowers/artifacts/p19/cluster-live-after.yaml
git add docs/superpowers/artifacts/p19/cluster-live-after.yaml
git commit -m "chore(p19): snapshot CNPG cluster post-rollout"
```

---

## Task 4: Dashboard + alert — replication lag panels, `>5s` sustained alert

**Files:**
- Modify: `tesserix-infra/grafana/dashboards/subscription-health.json` (if P17 has landed). Otherwise: create `tesserix-infra/grafana/dashboards/cnpg-replication.json` and note in the PR that it should be merged into subscription-health when P17 ships.
- Modify: `tesserix-infra/prometheus/rules/cnpg-alerts.yaml` (create file if absent).

**Spec references:** §22.2 (RPO=0 depends on standby keeping up).

- [ ] **Step 1: Add two panels**

Both are Prometheus time-series panels scraping CNPG's metrics (the operator emits `cnpg_pg_stat_replication_*` out of the box).

- Panel A: **Write lag (sync standby)** — promql:
  ```
  max by (pod) (cnpg_pg_stat_replication_write_lag_seconds{cluster="marketplace-db"})
  ```
  y-axis: seconds; threshold overlay at `5`.

- Panel B: **Replay lag (sync standby)** — promql:
  ```
  max by (pod) (cnpg_pg_stat_replication_replay_lag_seconds{cluster="marketplace-db"})
  ```
  y-axis: seconds; threshold overlay at `5`.

If operator exposes different metric names (newer CNPG versions renamed to `cnpg_backends_replication_lag_seconds`), verify via `curl http://<operator-metrics>/metrics | grep -i lag` before committing the JSON.

- [ ] **Step 2: Add alert rule**

Add to `cnpg-alerts.yaml`:

```yaml
groups:
  - name: cnpg-sync-standby
    rules:
      - alert: CNPGSyncStandbyLagHigh
        expr: |
          max(cnpg_pg_stat_replication_write_lag_seconds{cluster="marketplace-db"}) > 5
        for: 1m
        labels:
          severity: warning
          service: marketplace-api
        annotations:
          summary: "marketplace-db sync standby write lag >5s"
          description: |
            Sync standby is falling behind — commits may slow down (synchronous_commit=on
            waits for standby ack). If sustained, investigate standby pod resources,
            disk I/O, or network between AZs. Runbook: docs/runbooks/cnpg-primary-failover.md
          runbook_url: "https://github.com/tesserix/mark8ly/blob/main/docs/runbooks/cnpg-primary-failover.md"
```

- [ ] **Step 3: Commit**

```bash
git add tesserix-infra/grafana/dashboards/*.json tesserix-infra/prometheus/rules/cnpg-alerts.yaml
git commit -m "feat(observability): CNPG sync-standby lag panels + alert"
git push
```

ArgoCD picks it up via the monitoring ApplicationSet; verify in Grafana that both panels render with live data within 10 min.

---

## Task 5: Runbook + kill-primary drill + sign-off

**Files:**
- Create: `docs/runbooks/cnpg-primary-failover.md`.

**Spec references:** §22.2 (RTO 2 min at 100-merchant tier).

- [ ] **Step 1: Write the runbook**

Minimum contents, in this order:

1. **Scope** — one paragraph: marketplace-db primary failover, auto + manual paths, RTO target 2 min / RPO 0 per spec §22.2.
2. **Auto-failover flow**:
   - Kubernetes liveness probe fails on primary pod.
   - CNPG operator observes pod NotReady, promotes the synchronous standby to primary.
   - `marketplace-db-rw` Service endpoint is rewritten by the operator to point at the new primary.
   - GORM pool in marketplace-api sees `connection refused`, pool drops, new connections route to the new primary.
   - Target: service resumes serving writes within 120 s.
3. **Connection-string stability** — applications MUST use `marketplace-db-rw.platform.svc.cluster.local:5432`. Never pin to a pod IP. The `-rw` service is the stable contract.
4. **Manual promotion** (if auto-failover fails):
   ```bash
   kubectl cnpg promote -n platform marketplace-db marketplace-db-2
   # Wait for:
   kubectl get cluster -n platform marketplace-db -w
   # Confirm primaryPod has changed:
   kubectl get cluster -n platform marketplace-db -o jsonpath='{.status.currentPrimary}'
   ```
5. **Post-failover validation** — four queries on the new primary:
   ```sql
   -- 1. Confirm writer role
   SELECT pg_is_in_recovery();                 -- expect: false
   -- 2. Known row smoke check
   SELECT count(*) FROM store_subscriptions;   -- expect: non-zero, matches pre-failover
   -- 3. Replication re-established?
   SELECT application_name, sync_state
     FROM pg_stat_replication;                 -- expect: one row, sync_state=sync
   -- 4. Replication lag on new standby
   SELECT pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn) AS lag_bytes
     FROM pg_stat_replication;                 -- expect: small (<1 MiB)
   ```
6. **If replication does NOT recover** — standby stuck:
   ```bash
   kubectl cnpg status -n platform marketplace-db
   # If status shows missing standby, re-create it:
   kubectl delete pod -n platform marketplace-db-<old-primary>
   # CNPG will re-provision via base-backup. Expect ~10-30 min for 200Gi.
   ```
7. **Latency caveat** — `synchronous_commit=on` adds round-trip latency (~1–5 ms per commit within the same zone). This is ACCEPTED per spec §22 — churn risk at 100 merchants > the latency cost. If p95 write latency creeps past 500 ms sustained, page the on-call DB owner.
8. **Rollback** — to temporarily accept data-loss risk for availability (e.g. standby pod OOM-looping, blocking all writes), flip to async:
   ```yaml
   # spec.postgresql.synchronous:
   #   method: any
   #   number: 0       # zero standbys required → async
   ```
   Commit in tesserix-k8s. Understand that RPO is no longer 0 while this is in effect. Escalate.

- [ ] **Step 2: Drill in staging (NOT prod)**

```bash
# Identify current primary
PRIMARY=$(kubectl get cluster -n platform marketplace-db \
  -o jsonpath='{.status.currentPrimary}')
# Record t0
date -u +%s > /tmp/t0.txt
# Kill it
kubectl delete pod -n platform "$PRIMARY" --grace-period=0 --force
# Watch until new primary is healthy
kubectl get cluster -n platform marketplace-db -w
# When status.currentPrimary changes and readyInstances=2 again:
date -u +%s > /tmp/t1.txt
echo "elapsed: $(($(cat /tmp/t1.txt) - $(cat /tmp/t0.txt)))s"
```

Target: elapsed ≤ 120s. Record actual value in the PR description.

- [ ] **Step 3: Confirm marketplace-api resumed serving traffic**

In a second terminal during the drill, run:

```bash
while true; do
  curl -s -o /dev/null -w "%{http_code} %{time_total}\n" \
    https://<staging-marketplace-api>/health
  sleep 1
done
```

Expect a short window of non-200s (≤2 min), then steady 200s. Attach the captured output to the PR.

- [ ] **Step 4: Commit runbook + drill artifacts**

```bash
git add docs/runbooks/cnpg-primary-failover.md
git add docs/superpowers/artifacts/p19/drill-output.txt
git commit -m "docs(runbook): cnpg-primary-failover + staging drill evidence"
git push
```

- [ ] **Step 5: Sign-off checklist**

Before closing P19, verify all of:

- [ ] `kubectl get cluster -n platform marketplace-db -o jsonpath='{.status.instances}'` returns `2`
- [ ] `kubectl exec ... -- psql -c "SHOW synchronous_commit;"` returns `on`
- [ ] `pg_stat_replication` shows one row with `sync_state = sync`
- [ ] `marketplace-db-rw` / `-ro` / `-r` services all exist
- [ ] Grafana panels for write_lag + replay_lag render live data
- [ ] `CNPGSyncStandbyLagHigh` alert rule is loaded in Prometheus (`promtool check rules cnpg-alerts.yaml`)
- [ ] Runbook exists at `docs/runbooks/cnpg-primary-failover.md`
- [ ] Staging kill-primary drill measured RTO ≤ 120 s
- [ ] `spec.backup` hash matches pre-edit hash
- [ ] ArgoCD shows marketplace-api-db app Synced + Healthy

---

## Final verification

Single-shot script to run after all tasks:

```bash
#!/usr/bin/env bash
set -euo pipefail

ns=platform
cluster=marketplace-db

echo "== instances =="
kubectl get cluster -n "$ns" "$cluster" \
  -o jsonpath='instances={.status.instances} ready={.status.readyInstances}{"\n"}'

echo "== synchronous_commit =="
kubectl exec -n "$ns" "${cluster}-1" -- \
  psql -U postgres -tAc "SHOW synchronous_commit;"

echo "== pg_stat_replication =="
kubectl exec -n "$ns" "${cluster}-1" -- \
  psql -U postgres -c "SELECT application_name, sync_state, state FROM pg_stat_replication;"

echo "== services =="
kubectl get svc -n "$ns" | grep "$cluster"

echo "== marketplace-api health =="
curl -sSf "https://${MARKETPLACE_API_HOST}/health" | jq .
```

All five blocks must match expectations from Task 3. Any mismatch blocks P19 completion.

---

## What this unlocks

- **Public launch readiness** — spec §22 names sync standby at 100-merchant tier as a gate before public launch. With P19 landed, the DR contract in the public-launch checklist is satisfied.
- **RPO=0 / RTO 2 min** operational posture for marketplace-api's Postgres.
- **Foundation for P20 (PgBouncer)** — P20 sits in front of the same `Cluster` and benefits from the already-scaled primary + standby.
- **Foundation for async read replicas** at the 500–2,000 tier (next staircase step in §24).

---

## Key risks

1. **Commit latency** — `synchronous_commit = on` waits for standby fsync before acknowledging a write. Expect +1–5 ms per transaction within-zone, more cross-zone. Accepted per §22 (churn risk > ~$15/mo latency cost). Monitor p95 write latency after rollout.
2. **Online scale-up from 1 → 2 instances** — CNPG handles it automatically via streaming base-backup of the primary. Validate in staging first (Task 5 Step 2). 200 GiB base-backup can take 10–30 min; Grafana should show the second pod catching up before the standby is declared `sync`.
3. **Operator version** — `spec.postgresql.synchronous` requires CNPG ≥1.24 (Task 1 Step 5). Older operators silently ignore the block, leaving the cluster at async replication with `synchronous_commit=on` doing nothing (worst of both worlds — latency cost without RPO guarantee). This check is why Task 1 Step 5 exists.
4. **Backup/WAL config drift** — an errant YAML edit could silently drop `spec.backup` rules, breaking PITR. The hash check at Task 1 Step 4 → Task 2 Step 3 is the guard.
5. **Standby OOM under burst load** — new standby pod sized to `1 CPU / 4 GiB`. If it OOMs, commits block. Alert `CNPGSyncStandbyLagHigh` catches this within 1 min; rollback path in the runbook (flip to `number: 0`) is the pressure valve.

---

## Execution handoff

Start a fresh worktree:

```bash
git worktree add ../mark8ly-p19 -b p19/cnpg-sync-standby main
cd ../mark8ly-p19
```

Then hand off to the executing-plans sub-skill with this file as input. The plan is infra-YAML + runbook only, so no test matrix, no lint gate beyond `kubeconform` + `promtool check rules`. Completion evidence lives under `docs/superpowers/artifacts/p19/`.
