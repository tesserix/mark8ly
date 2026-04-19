# Runbook — CNPG primary failover (mark8ly-postgres)

**Scope:** `mark8ly-postgres` CNPG cluster (namespace `mark8ly`). Hosts
`mark8ly_platform_api`, `mark8ly_openfga`, and `mark8ly_marketplace_api`
databases. Covers automated failover after a primary pod loss and the
manual-promotion fallback.

**Targets** (spec §22.2, 100-merchant tier):

| Metric | Target |
|---|---|
| RPO (data loss) | 0 — `synchronous_commit = on` + `synchronous.number: 1` |
| RTO (time to serve writes again) | ≤ 2 minutes |

**Preconditions** verified in every drill:

- `kubectl get cluster -n mark8ly mark8ly-postgres` shows `instances: 2` and `readyInstances: 2`.
- `kubectl exec -n mark8ly mark8ly-postgres-1 -c postgres -- psql -U postgres -tAc "SHOW synchronous_commit;"` returns `on`.
- `pg_stat_replication` has exactly one row with `sync_state = sync` and `state = streaming`.

If any precondition fails, the failover guarantees below don't hold. STOP
and restore them before continuing.

---

## 1. Auto-failover flow (expected path)

1. Primary pod's Kubernetes liveness probe fails (crash, OOMKill, node
   eviction, PVC detach).
2. CNPG operator observes the pod `NotReady` → initiates promotion of
   the synchronous standby.
3. Operator rewrites the `mark8ly-postgres-rw` Service endpoint to the
   new primary pod.
4. `marketplace-api` pods see `connection refused` on their GORM pool,
   drop existing connections, open new ones via the `-rw` Service, and
   resume serving traffic.

**Target end-to-end time:** ≤ 120 seconds.

### Connection-string stability (non-negotiable)

Every service **MUST** connect via the CNPG-managed Services:

```
Writer (primary only):         mark8ly-postgres-rw.mark8ly.svc.cluster.local:5432
Read-only (standbys only):     mark8ly-postgres-ro.mark8ly.svc.cluster.local:5432
Any replica (primary + standby): mark8ly-postgres-r.mark8ly.svc.cluster.local:5432
```

Never pin a pod IP or podname. The Service is the stable contract — the
operator rewrites its endpoint on failover; pod IPs are ephemeral.

---

## 2. Verifying auto-failover succeeded

```bash
# Current primary pod:
kubectl get cluster -n mark8ly mark8ly-postgres \
  -o jsonpath='{.status.currentPrimary}{"\n"}'

# Service endpoint points at it:
kubectl get endpoints -n mark8ly mark8ly-postgres-rw \
  -o jsonpath='{.subsets[0].addresses[0].ip}{"\n"}'

# Replica status (on the NEW primary):
kubectl exec -n mark8ly mark8ly-postgres-1 -c postgres -- \
  psql -U postgres -c "SELECT application_name, sync_state, state FROM pg_stat_replication;"
```

Expect:

- `currentPrimary` differs from the pre-failover value (recorded from the
  `t0` snapshot in §4 below).
- Endpoint IP matches the new primary pod IP.
- Exactly one `pg_stat_replication` row with `sync_state = sync`.

---

## 3. Manual promotion (auto-failover stuck)

If the operator does not promote within 2 minutes (e.g. both pods
`NotReady`, operator misbehaving, network partition):

```bash
# Identify the healthy standby pod (usually mark8ly-postgres-2):
STANDBY=$(kubectl get pod -n mark8ly -l cnpg.io/cluster=mark8ly-postgres \
  -o jsonpath='{range .items[?(@.status.phase=="Running")]}{.metadata.name}{"\n"}{end}' \
  | grep -v "$(kubectl get cluster -n mark8ly mark8ly-postgres \
       -o jsonpath='{.status.currentPrimary}')" \
  | head -1)
echo "Promoting: $STANDBY"

# Force promote:
kubectl cnpg promote -n mark8ly mark8ly-postgres "$STANDBY"

# Watch until complete (Ctrl-C when .status.currentPrimary flips):
kubectl get cluster -n mark8ly mark8ly-postgres -w
```

---

## 4. Post-failover validation (always run)

All four queries run against the **new primary** via the `-rw` Service or
directly via `kubectl exec`:

```sql
-- 1. New primary is actually a writer (not in recovery).
SELECT pg_is_in_recovery();                 -- expect: false

-- 2. Known-row smoke check. Pick a table with low churn.
SELECT count(*) FROM store_subscriptions;   -- expect: non-zero,
                                            -- matches pre-failover within
                                            -- the in-flight-write delta.

-- 3. New replication pair established.
SELECT application_name, sync_state, state
  FROM pg_stat_replication;                 -- expect: one row, sync_state=sync

-- 4. Lag on the new standby (former primary OR a re-provisioned one).
SELECT pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn) AS lag_bytes
  FROM pg_stat_replication;                 -- expect: small (<1 MiB)
```

**Record wall-clock elapsed** from first-alert to all-four-green. That
number is the realised RTO and should be ≤ 120 s.

---

## 5. Standby not recovering (warm-standby stuck)

If `pg_stat_replication` stays empty after §4 for longer than ~30 minutes
(the 200 GiB base-backup is slow but should not hang indefinitely):

```bash
kubectl cnpg status -n mark8ly mark8ly-postgres
# Look for Phase / Ready / Role columns — "Replica init failed" or
# "Base backup timed out" indicate the standby can't catch up.

# Re-provisioning: delete the failing pod; CNPG re-runs base-backup.
kubectl delete pod -n mark8ly <failing-standby-pod-name>

# Expect 10–30 minutes on 200 GiB PVC. Grafana panel
# cnpg_pg_stat_replication_write_lag_seconds will drop from +Inf to 0
# when the new standby reaches sync state.
```

Until replication is re-established, the cluster runs with effectively
**zero** synchronous acks. In CNPG's behaviour with `synchronous.number:
1` and zero standbys, commits do NOT block — they fall through to local.
**This silently violates RPO=0.** The `CNPGSyncStandbyMissing` alert
fires at this state (see `k8s/cluster/prometheus/rules/cnpg.yaml`).

---

## 6. Latency caveats (expected, not a bug)

`synchronous_commit = on` adds +1–5 ms per commit within-AZ (fsync
round-trip), more cross-AZ. Spec §22 accepts this cost: the churn risk at
100 merchants outweighs the latency budget.

If p95 write latency sustains above 500 ms:

1. Check `CNPGSyncStandbyLagHigh` + `CNPGSyncStandbyReplayLagHigh` alerts.
2. Check standby pod CPU + memory pressure — bursts under the 1 CPU / 4
   GiB cap will bottleneck replay.
3. Check PVC IOPS on the standby — GKE PD-SSD is fine under normal load,
   but a `pg_dump` or ad-hoc analytical query running against the standby
   can starve WAL replay.
4. Last resort — follow the rollback in §7 to temporarily drop to async,
   alerting the DB owner first.

---

## 7. Emergency rollback — flip to async

Use only when the standby is OOM-looping or otherwise blocking all writes
and availability is more important than RPO=0.

Edit `tesserix-k8s/charts/apps/mark8ly-postgres/values.yaml`:

```yaml
# BEFORE (RPO=0, synchronous):
instances: 2
# (template adds spec.postgresql.synchronous.{method: any, number: 1})

# AFTER — scale down to 1 instance; synchronous block removed by template:
instances: 1
```

Commit, push, ArgoCD auto-syncs within ~2 min. The template's `{{- if gt
(int .Values.instances) 1 }}` guard drops the `spec.postgresql.synchronous`
block, so CNPG stops requiring standby ACKs. `synchronous_commit: "on"`
remains in parameters but becomes inert (no standby names registered).

**Understand**: for the duration this is in effect, writes committed after
the flip are not durable against primary loss. Page the DB owner before
and after. File a rollback ticket with the reason and expected mitigation.

Reverse path: bump `instances` back to 2, wait for base-backup, verify §4.

---

## 8. Sign-off checklist (post-drill / post-real-failover)

Before closing the incident:

- [ ] `kubectl get cluster -n mark8ly mark8ly-postgres -o jsonpath='{.status.instances}'` returns `2`
- [ ] `kubectl exec ... -- psql -c "SHOW synchronous_commit;"` returns `on`
- [ ] `pg_stat_replication` shows one row with `sync_state = sync`
- [ ] `-rw` / `-ro` / `-r` Services all exist and have endpoints
- [ ] Grafana dashboard (TBD — P17 + P19 Task 4 cnpg-replication panel when
      landed) shows write_lag and replay_lag trending to 0
- [ ] `CNPGSyncStandbyLagHigh` + `CNPGSyncStandbyMissing` rules loaded
      (`kubectl get prometheusrule -n monitoring cnpg-sync-standby-rules`)
- [ ] ArgoCD shows `mark8ly-postgres` app `Synced + Healthy`
- [ ] Realised RTO recorded in the incident ticket and compared to the
      ≤ 120 s target.

---

## 9. Staging drill procedure (kill-primary)

Run against **staging only**. Do not run against prod without a change
window + on-call coverage.

```bash
# Identify current primary
PRIMARY=$(kubectl get cluster -n mark8ly mark8ly-postgres \
  -o jsonpath='{.status.currentPrimary}')
echo "Primary before drill: $PRIMARY"

# Mark t0
date -u +%s > /tmp/t0.txt

# Kill it forcefully (simulates crash, not graceful shutdown)
kubectl delete pod -n mark8ly "$PRIMARY" --grace-period=0 --force

# Watch until new primary is healthy
kubectl get cluster -n mark8ly mark8ly-postgres -w
# Ctrl-C when:
#   - status.currentPrimary != "$PRIMARY"
#   - status.readyInstances == 2

# Mark t1
date -u +%s > /tmp/t1.txt
echo "Elapsed: $(($(cat /tmp/t1.txt) - $(cat /tmp/t0.txt))) seconds"
```

While the drill runs, poll marketplace-api health in a second terminal:

```bash
while true; do
  curl -s -o /dev/null -w "%{http_code} %{time_total}\n" \
    https://<staging-marketplace-api>/health
  sleep 1
done
```

Expect a short window of non-200s (≤ 2 min), then steady 200s. Attach the
capture to the PR description.

---

## References

- Spec `docs/superpowers/specs/2026-04-17-subscription-model-design.md`
  §22 (DR context), §22.2 (RTO/RPO table), §24 (CNPG staircase,
  "SA finding: synchronous_commit = on" callout).
- Plan `docs/superpowers/plans/2026-04-18-p19-cnpg-sync-standby.md`.
- Alert rules `tesserix-k8s/k8s/cluster/prometheus/rules/cnpg.yaml`.
- Cluster chart `tesserix-k8s/charts/apps/mark8ly-postgres/`.
- CNPG docs: <https://cloudnative-pg.io/documentation/current/failover/>.
