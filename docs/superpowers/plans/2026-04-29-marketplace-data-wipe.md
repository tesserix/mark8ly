# Marketplace Data Wipe — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Operational note:** This is a destructive prod-ops runbook, not a feature implementation. There are no failing tests to write — every "step" is either a read-only verify or a single destructive command. The TDD shape is replaced by **verify-act-verify**: read state → run command → read state again. STOP at the first unexpected output and surface to the user.

**Goal:** Wipe all test data from the Mark8ly prod environment so the platform is a clean v1 ready for real customers, while preserving schema, disks, infra, secrets, and reference data.

**Architecture:** Surgical TRUNCATE preserving schema + reference data on 3 Postgres DBs; targeted delete on MongoDB / Redis / GCS / GIP / Cloudflare. Apps quiesced for the wipe window, OpenFGA store re-created via existing init Job, session secret rotated to invalidate stale cookies.

**Tech Stack:** kubectl, psql (CNPG), mongosh, redis-cli, gcloud, gcloud Identity Toolkit REST API, ArgoCD CLI.

**Spec:** `docs/superpowers/specs/2026-04-29-marketplace-data-wipe-design.md`

**Pre-conditions verified before starting:**
- kubectl context = `gke_tesseracthub-480811_asia-south1_tesseract-prod-in-gke`
- gcloud project = `tesseracthub-480811`
- Logged-in gcloud user has Identity Platform Admin + Storage Admin roles
- Argocd CLI logged in, OR fall back to `kubectl annotate ... refresh=hard`

**Total wall-clock estimate:** 12-15 min (excluding manual smoke test).

---

## Task 0: Pre-flight verification (read-only)

**Files:** none. Pure read-only sanity checks before any destructive op.

- [ ] **Step 0.1: Verify kubectl context**

  Run:
  ```
  kubectl config current-context
  ```
  Expected output exactly:
  ```
  gke_tesseracthub-480811_asia-south1_tesseract-prod-in-gke
  ```
  STOP if anything else.

- [ ] **Step 0.2: Verify gcloud project**

  Run:
  ```
  gcloud config get-value project
  ```
  Expected: `tesseracthub-480811`. STOP otherwise.

- [ ] **Step 0.3: Re-confirm Pub/Sub has zero subscriptions on marketplace topics**

  Run:
  ```
  gcloud pubsub subscriptions list --project=tesseracthub-480811 \
    --format='value(name)' | grep -iE 'mark8ly|/mp-' || echo "no subs — OK"
  ```
  Expected: `no subs — OK`. STOP and reassess if any subscriptions appear (they would write back into wiped state).

- [ ] **Step 0.4: Snapshot the row counts we expect to zero**

  Run (per DB) and copy output for later diff:
  ```
  for DB in mark8ly_marketplace_api mark8ly_platform_api mark8ly_openfga; do
    echo "=== $DB ==="
    kubectl exec -n mark8ly mark8ly-postgres-2 -c postgres -- \
      psql -U postgres -d "$DB" -t -c \
      "SELECT relname, n_live_tup FROM pg_stat_user_tables WHERE n_live_tup > 0 ORDER BY n_live_tup DESC;"
  done
  ```
  Save output to `/tmp/mark8ly-prewipe-rowcounts.txt` so post-wipe verification has a baseline.

- [ ] **Step 0.5: Snapshot GIP user counts**

  Run:
  ```
  for T in MP-Customer-39opy MP-Internal-e986p Platform-9bu14; do
    echo "=== $T ==="
    gcloud identity-platform users list --tenant="$T" \
      --project=tesseracthub-480811 --format='value(uid)' | wc -l
  done
  ```
  Expected (approximate): 6, 180, 2.

---

## Task 1: Quiesce the system (Section 1 of spec)

**Files:** none — kubectl mutations only.

- [ ] **Step 1.1: Suspend the every-2-min sync CronJob**

  Run:
  ```
  kubectl patch cronjob mark8ly-marketplace-api-admin-tracking-sync -n mark8ly \
    --type merge -p '{"spec":{"suspend":true}}'
  ```
  Verify:
  ```
  kubectl get cronjob mark8ly-marketplace-api-admin-tracking-sync -n mark8ly \
    -o jsonpath='{.spec.suspend}'
  ```
  Expected: `true`.

- [ ] **Step 1.2: Suspend the CNPG ScheduledBackup**

  Run:
  ```
  kubectl patch scheduledbackup mark8ly-postgres-scheduled-backup -n mark8ly \
    --type merge -p '{"spec":{"suspend":true}}'
  ```
  Verify:
  ```
  kubectl get scheduledbackup mark8ly-postgres-scheduled-backup -n mark8ly \
    -o jsonpath='{.spec.suspend}'
  ```
  Expected: `true`.

- [ ] **Step 1.3: Scale Go service deployments to zero**

  Run:
  ```
  kubectl scale deployment -n mark8ly --replicas=0 \
    mark8ly-marketplace-api-admin \
    mark8ly-marketplace-api-storefront \
    mark8ly-platform-api \
    mark8ly-auth-bff
  ```
  Wait for pods to terminate:
  ```
  kubectl wait --for=delete pod -n mark8ly \
    -l app.kubernetes.io/name=mark8ly-marketplace-api-admin --timeout=60s
  kubectl wait --for=delete pod -n mark8ly \
    -l app.kubernetes.io/name=mark8ly-marketplace-api-storefront --timeout=60s
  kubectl wait --for=delete pod -n mark8ly \
    -l app.kubernetes.io/name=mark8ly-platform-api --timeout=60s
  kubectl wait --for=delete pod -n mark8ly \
    -l app.kubernetes.io/name=mark8ly-auth-bff --timeout=60s
  ```
  Verify all gone:
  ```
  kubectl get pods -n mark8ly | grep -E 'marketplace-api|platform-api|auth-bff'
  ```
  Expected: empty output (no rows).

---

## Task 2: Wipe Postgres (Section 2 of spec)

**Files:** none — psql DO blocks executed via kubectl exec.

For each DB the pattern is: read schema → preview wipe → execute → verify zero.

### 2A: `mark8ly_marketplace_api`

- [ ] **Step 2A.1: Preview which tables will be wiped vs preserved**

  Run:
  ```
  kubectl exec -n mark8ly mark8ly-postgres-2 -c postgres -- \
    psql -U postgres -d mark8ly_marketplace_api -c \
    "SELECT tablename,
            CASE WHEN tablename IN ('schema_migrations','schema_marker')
                 THEN 'PRESERVE' ELSE 'WIPE' END AS action
       FROM pg_tables WHERE schemaname='public' ORDER BY action, tablename;"
  ```
  STOP if any table named `*reference*`, `*config*`, `*seed*`, `*country*`, `*currency*`, `*timezone*`, `*state*` shows action=WIPE — investigate before proceeding.

- [ ] **Step 2A.2: Execute TRUNCATE loop**

  Run:
  ```
  kubectl exec -n mark8ly mark8ly-postgres-2 -c postgres -- \
    psql -U postgres -d mark8ly_marketplace_api -c "
  DO \$\$ DECLARE r RECORD;
  BEGIN
    FOR r IN SELECT tablename FROM pg_tables
             WHERE schemaname='public'
               AND tablename NOT IN ('schema_migrations','schema_marker')
    LOOP
      EXECUTE format('TRUNCATE TABLE public.%I RESTART IDENTITY CASCADE', r.tablename);
    END LOOP;
  END \$\$;"
  ```
  Expected: `DO` (no error).

- [ ] **Step 2A.3: Verify zero rows in non-preserve tables**

  Run:
  ```
  kubectl exec -n mark8ly mark8ly-postgres-2 -c postgres -- \
    psql -U postgres -d mark8ly_marketplace_api -t -c \
    "SELECT relname, n_live_tup FROM pg_stat_user_tables
      WHERE n_live_tup > 0 ORDER BY n_live_tup DESC;"
  ```
  Expected: empty result. STOP if any table has rows.

### 2B: `mark8ly_platform_api`

- [ ] **Step 2B.1: Preview which tables will be wiped vs preserved**

  Run:
  ```
  kubectl exec -n mark8ly mark8ly-postgres-2 -c postgres -- \
    psql -U postgres -d mark8ly_platform_api -c \
    "SELECT tablename,
            CASE WHEN tablename IN ('schema_migrations','schema_marker','countries','states','currencies','timezones')
                 THEN 'PRESERVE' ELSE 'WIPE' END AS action
       FROM pg_tables WHERE schemaname='public' ORDER BY action, tablename;"
  ```
  STOP if any table named `*reference*`, `*config*`, `*seed*` (other than the 4 already in PRESERVE) shows action=WIPE — investigate.

- [ ] **Step 2B.2: Execute TRUNCATE loop with reference-data preserve**

  Run:
  ```
  kubectl exec -n mark8ly mark8ly-postgres-2 -c postgres -- \
    psql -U postgres -d mark8ly_platform_api -c "
  DO \$\$ DECLARE r RECORD;
  BEGIN
    FOR r IN SELECT tablename FROM pg_tables
             WHERE schemaname='public'
               AND tablename NOT IN ('schema_migrations','schema_marker','countries','states','currencies','timezones')
    LOOP
      EXECUTE format('TRUNCATE TABLE public.%I RESTART IDENTITY CASCADE', r.tablename);
    END LOOP;
  END \$\$;"
  ```
  Expected: `DO`.

- [ ] **Step 2B.3: Verify only preserve-list tables have rows**

  Run:
  ```
  kubectl exec -n mark8ly mark8ly-postgres-2 -c postgres -- \
    psql -U postgres -d mark8ly_platform_api -t -c \
    "SELECT relname, n_live_tup FROM pg_stat_user_tables
      WHERE n_live_tup > 0 ORDER BY relname;"
  ```
  Expected exactly (counts approximate):
  ```
  countries  | 20
  currencies | 16
  states     | 117
  timezones  | 22
  ```
  STOP if any other table appears.

### 2C: `mark8ly_openfga`

- [ ] **Step 2C.1: Wipe everything except schema_migrations**

  Run:
  ```
  kubectl exec -n mark8ly mark8ly-postgres-2 -c postgres -- \
    psql -U postgres -d mark8ly_openfga -c "
  DO \$\$ DECLARE r RECORD;
  BEGIN
    FOR r IN SELECT tablename FROM pg_tables
             WHERE schemaname='public'
               AND tablename NOT IN ('schema_migrations')
    LOOP
      EXECUTE format('TRUNCATE TABLE public.%I RESTART IDENTITY CASCADE', r.tablename);
    END LOOP;
  END \$\$;"
  ```

- [ ] **Step 2C.2: Verify openfga is empty**

  Run:
  ```
  kubectl exec -n mark8ly mark8ly-postgres-2 -c postgres -- \
    psql -U postgres -d mark8ly_openfga -c \
    "SELECT (SELECT count(*) FROM store) AS stores,
            (SELECT count(*) FROM tuple) AS tuples,
            (SELECT count(*) FROM authorization_model) AS models;"
  ```
  Expected: `0 | 0 | 0`.

### 2D: Re-create OpenFGA store via init Job

- [ ] **Step 2D.1: Delete existing fga-init Job**

  Run:
  ```
  kubectl delete job -n mark8ly -l app=mark8ly-fga-init --ignore-not-found
  ```

- [ ] **Step 2D.2: Trigger ArgoCD to re-create the Job**

  Try argocd CLI first:
  ```
  argocd app sync mark8ly-fga-init --force --replace
  ```
  If `argocd` CLI not configured, fall back:
  ```
  kubectl -n argocd annotate application mark8ly-fga-init \
    argocd.argoproj.io/refresh=hard --overwrite
  ```

- [ ] **Step 2D.3: Wait for the new Job to complete**

  Run:
  ```
  kubectl wait --for=condition=complete -n mark8ly \
    job -l app=mark8ly-fga-init --timeout=120s
  ```
  Expected: `condition met` within 2 min. STOP and inspect logs if it times out:
  ```
  kubectl logs -n mark8ly -l app=mark8ly-fga-init --tail=50
  ```

- [ ] **Step 2D.4: Confirm exactly 1 OpenFGA store named `mark8ly-platform`**

  Run:
  ```
  kubectl exec -n mark8ly mark8ly-postgres-2 -c postgres -- \
    psql -U postgres -d mark8ly_openfga -c \
    "SELECT id, name FROM store;"
  ```
  Expected: 1 row with `name = 'mark8ly-platform'`.

  Also confirm an authorization_model was written:
  ```
  kubectl exec -n mark8ly mark8ly-postgres-2 -c postgres -- \
    psql -U postgres -d mark8ly_openfga -c \
    "SELECT count(*) FROM authorization_model;"
  ```
  Expected: `1`.

---

## Task 3: Wipe external systems (Section 3 of spec)

### 3A: Redis

- [ ] **Step 3A.1: Get Redis password**

  Run:
  ```
  REDIS_PASS=$(kubectl get secret -n redis-marketplace redis \
    -o jsonpath='{.data.password}' | base64 -d)
  ```
  Verify non-empty: `echo "${#REDIS_PASS}"` > 0.

- [ ] **Step 3A.2: FLUSHALL**

  Run:
  ```
  kubectl exec -n redis-marketplace redis-0 -- redis-cli -a "$REDIS_PASS" FLUSHALL
  ```
  Expected: `OK`.

- [ ] **Step 3A.3: Verify empty**

  Run:
  ```
  kubectl exec -n redis-marketplace redis-0 -- redis-cli -a "$REDIS_PASS" DBSIZE
  ```
  Expected: `(integer) 0`.

### 3B: MongoDB

- [ ] **Step 3B.1: Drop the otto database**

  Run:
  ```
  kubectl exec -n mark8ly mark8ly-mongodb-0 -- mongosh --quiet --eval \
    'db.getSiblingDB("otto").dropDatabase()'
  ```
  Expected: `{ ok: 1, dropped: 'otto' }`.

- [ ] **Step 3B.2: Verify otto is gone**

  Run:
  ```
  kubectl exec -n mark8ly mark8ly-mongodb-0 -- mongosh --quiet --eval \
    'db.adminCommand({listDatabases:1}).databases.map(d=>d.name)'
  ```
  Expected: `otto` is NOT in the list (only admin/config/local).

### 3C: GCS

- [ ] **Step 3C.1: Confirm bucket has objects (preview)**

  Run:
  ```
  gcloud storage ls 'gs://tesseracthub-480811-mark8ly-media/**' \
    --project=tesseracthub-480811 | wc -l
  ```
  Expected: > 0.

- [ ] **Step 3C.2: Recursive delete bucket contents (keep bucket)**

  Run:
  ```
  gcloud storage rm -r 'gs://tesseracthub-480811-mark8ly-media/**' \
    --project=tesseracthub-480811
  ```
  Expected: rm completes, no error.

- [ ] **Step 3C.3: Verify empty**

  Run:
  ```
  gcloud storage ls 'gs://tesseracthub-480811-mark8ly-media/' \
    --project=tesseracthub-480811 | wc -l
  ```
  Expected: `0`. The bucket itself still exists:
  ```
  gcloud storage buckets describe gs://tesseracthub-480811-mark8ly-media \
    --format='value(name)' --project=tesseracthub-480811
  ```
  Expected: `tesseracthub-480811-mark8ly-media`.

### 3D: GIP users (3 active tenants)

- [ ] **Step 3D.1: Define helper function (paste once into shell)**

  ```
  wipe_gip_tenant() {
    local T="$1"
    local uids
    uids=$(gcloud identity-platform users list --tenant="$T" \
      --project=tesseracthub-480811 --format='value(uid)')
    if [[ -z "$uids" ]]; then
      echo "[$T] already empty"
      return
    fi
    local arr=( $uids )
    echo "[$T] deleting ${#arr[@]} users"
    local i
    for ((i=0; i<${#arr[@]}; i+=100)); do
      local chunk=( "${arr[@]:i:100}" )
      local json
      json=$(printf '%s\n' "${chunk[@]}" | jq -R . | jq -s .)
      curl -fsS -X POST \
        -H "Authorization: Bearer $(gcloud auth print-access-token)" \
        -H "Content-Type: application/json" \
        -d "{\"localIds\":${json},\"force\":true}" \
        "https://identitytoolkit.googleapis.com/v1/projects/tesseracthub-480811/tenants/$T/accounts:batchDelete" \
        | jq .
    done
  }
  ```

- [ ] **Step 3D.2: Wipe MP-Customer-39opy (≈6 users)**

  Run: `wipe_gip_tenant MP-Customer-39opy`
  Verify: `gcloud identity-platform users list --tenant=MP-Customer-39opy --project=tesseracthub-480811 --limit=1 --format='value(uid)'` → empty.

- [ ] **Step 3D.3: Wipe MP-Internal-e986p (≈180 users, 2 batches)**

  Run: `wipe_gip_tenant MP-Internal-e986p`
  Verify same way → empty.

- [ ] **Step 3D.4: Wipe Platform-9bu14 (≈2 users)**

  Run: `wipe_gip_tenant Platform-9bu14`
  Verify same way → empty.

### 3E: Delete GIP zombie tenants

- [ ] **Step 3E.1: Delete the 3 empty dupes**

  Run:
  ```
  for T in MP-Customer-zoe11 MP-Internal-z5rnh Platform-2c9z0; do
    gcloud identity-platform tenants delete "$T" \
      --project=tesseracthub-480811 --quiet
  done
  ```

- [ ] **Step 3E.2: Verify only 3 tenants remain**

  Run:
  ```
  gcloud identity-platform tenants list --project=tesseracthub-480811 \
    --format='value(name)'
  ```
  Expected: exactly `MP-Customer-39opy`, `MP-Internal-e986p`, `Platform-9bu14`.

### 3F: Cloudflare DNS (manual)

- [ ] **Step 3F.1: Manually remove `primasyss.com` CNAME**

  Action: log into Cloudflare dashboard for the `primasyss.com` zone (or wherever the CNAME lives), find the record pointing at the storefront ingress, delete it.

  Verify (from local machine):
  ```
  dig +short primasyss.com CNAME
  dig +short primasyss.com A
  ```
  Expected: empty / NXDOMAIN within DNS TTL.

---

## Task 4: Rotate session secret (eliminates stale-cookie risk)

**Files:** none.

- [ ] **Step 4.1: Add a new version to the GCP secret with fresh entropy**

  Run:
  ```
  gcloud secrets versions add mark8ly-prod-session \
    --data-file=<(openssl rand -base64 32) \
    --project=tesseracthub-480811
  ```
  Expected: `Created version [N] of the secret [mark8ly-prod-session]`.

- [ ] **Step 4.2: Force ExternalSecrets to sync the new version**

  Run:
  ```
  kubectl annotate externalsecret mark8ly-session -n mark8ly \
    force-sync=$(date +%s) --overwrite
  ```
  Wait ~10s, then verify the K8s Secret was updated:
  ```
  kubectl get secret mark8ly-session -n mark8ly \
    -o jsonpath='{.metadata.annotations}' | jq .
  ```
  Expected: `reconcile.external-secrets.io/data-hash` differs from before.

---

## Task 5: Restart apps + final verification (Section 4 of spec)

### 5A: Resume background jobs

- [ ] **Step 5A.1: Un-suspend tracking-sync CronJob**

  Run:
  ```
  kubectl patch cronjob mark8ly-marketplace-api-admin-tracking-sync -n mark8ly \
    --type merge -p '{"spec":{"suspend":false}}'
  ```

- [ ] **Step 5A.2: Un-suspend ScheduledBackup**

  Run:
  ```
  kubectl patch scheduledbackup mark8ly-postgres-scheduled-backup -n mark8ly \
    --type merge -p '{"spec":{"suspend":false}}'
  ```

### 5B: Scale apps back up

- [ ] **Step 5B.1: Scale all 4 deployments to 1**

  Run:
  ```
  kubectl scale deployment -n mark8ly --replicas=1 \
    mark8ly-marketplace-api-admin \
    mark8ly-marketplace-api-storefront \
    mark8ly-platform-api \
    mark8ly-auth-bff
  ```

- [ ] **Step 5B.2: Wait for all to be Ready**

  Run:
  ```
  for D in mark8ly-marketplace-api-admin mark8ly-marketplace-api-storefront \
           mark8ly-platform-api mark8ly-auth-bff; do
    kubectl rollout status deployment/$D -n mark8ly --timeout=180s
  done
  ```
  Expected: all 4 print `successfully rolled out`.

### 5C: Health checks

- [ ] **Step 5C.1: Check /health for each Go service**

  Run:
  ```
  for SVC in mark8ly-marketplace-api-admin mark8ly-marketplace-api-storefront \
             mark8ly-platform-api mark8ly-auth-bff; do
    POD=$(kubectl get pod -n mark8ly -l app.kubernetes.io/name=$SVC \
      -o jsonpath='{.items[0].metadata.name}')
    echo "=== $SVC ($POD) ==="
    kubectl exec -n mark8ly "$POD" -c server -- \
      wget -qO- http://localhost:8080/health || echo FAILED
  done
  ```
  Expected: each returns 200/`ok`-style response. STOP on any FAILED.

- [ ] **Step 5C.2: Verify marketplace-api discovered the new OpenFGA store**

  Run:
  ```
  kubectl logs -n mark8ly -l app.kubernetes.io/name=mark8ly-marketplace-api-admin \
    --tail=200 | grep -iE 'fga|openfga|store'
  ```
  Expected: a log line confirming store discovery (something like `discovered FGA store id=01...`). STOP if logs show repeated retry/error.

---

## Task 6: End-to-end smoke test (manual — proves onboarding works)

**This task is run by the human, not automated.** Execute steps in a fresh incognito browser session.

- [ ] **Step 6.1:** Open `https://mark8ly.com/sign-up`. Page loads, no console errors, no 5xx.

- [ ] **Step 6.2:** Sign up with a real email. Receive verification email. Verify.

- [ ] **Step 6.3:** Complete onboarding wizard (store name, slug, country). Land on `<slug>-admin.mark8ly.com`.

- [ ] **Step 6.4:** Verify DB rows appeared:
  ```
  kubectl exec -n mark8ly mark8ly-postgres-2 -c postgres -- \
    psql -U postgres -d mark8ly_platform_api -c \
    "SELECT id, slug, name FROM tenants; SELECT id, tenant_id, slug FROM stores;"
  ```
  Expected: exactly 1 tenant, exactly 1 store.

- [ ] **Step 6.5:** Verify OpenFGA tuples appeared:
  ```
  kubectl exec -n mark8ly mark8ly-postgres-2 -c postgres -- \
    psql -U postgres -d mark8ly_openfga -c \
    "SELECT object_type, object_id, relation, _user FROM tuple LIMIT 10;"
  ```
  Expected: at least one `tenant:<id>#owner@user:<uid>` and one `store:<id>#owner@user:<uid>`.

- [ ] **Step 6.6:** In admin, create a product and upload an image. Verify GCS:
  ```
  gcloud storage ls 'gs://tesseracthub-480811-mark8ly-media/**' \
    --project=tesseracthub-480811
  ```
  Expected: at least 1 object under `tenants/<tid>/products/`.

- [ ] **Step 6.7:** Visit `https://<slug>.mark8ly.com` (incognito, no cookies). Confirm the product is visible.

If all 7 pass: wipe is complete, platform is a clean v1.

---

## Task 7: Final sign-off

- [ ] **Step 7.1: Diff the row-count snapshot**

  Run:
  ```
  for DB in mark8ly_marketplace_api mark8ly_platform_api mark8ly_openfga; do
    echo "=== $DB ==="
    kubectl exec -n mark8ly mark8ly-postgres-2 -c postgres -- \
      psql -U postgres -d "$DB" -t -c \
      "SELECT relname, n_live_tup FROM pg_stat_user_tables WHERE n_live_tup > 0 ORDER BY n_live_tup DESC;"
  done > /tmp/mark8ly-postwipe-rowcounts.txt
  diff /tmp/mark8ly-prewipe-rowcounts.txt /tmp/mark8ly-postwipe-rowcounts.txt | head -100
  ```
  Confirm only the 4 reference tables (countries/states/currencies/timezones in platform_api) plus the 1 newly-created OpenFGA store + tuples (from smoke test) + any rows from the smoke-test signup remain. Everything else should be gone.

- [ ] **Step 7.2: Commit any wipe artifacts**

  No code was changed in this plan; the design doc and this plan doc are already committed. If the wipe surfaced anything worth recording (e.g., a reference table missed by the preserve list), open a follow-up issue or memory entry.

---

## Rollback notes

This plan does NOT include rollback because the user explicitly opted out of taking a backup. If something goes catastrophically wrong:

- **App down but DB intact:** scale apps back up via Step 5B; wipe state may be partial but app should still boot
- **DB corruption / wipe failed mid-loop:** the `DO $$ ... $$` block runs in an implicit transaction; either it all committed or it all rolled back. Re-run the offending step.
- **Total loss:** CNPG has barman archive in GCS bucket `tesseracthub-480811-mark8ly-pg-backups` with PITR. Manually invoke recovery via CNPG `Backup` + `Cluster.spec.bootstrap.recovery` — outside this plan's scope.

## Out-of-scope follow-ups (track separately)

- Clean up 781 stale CNPG `Backup` CRs in `mark8ly` ns
- Investigate 2 OutOfSync ArgoCD apps (`mark8ly`, `mark8ly-postgres`)
- Delete 3 empty stale GCS buckets (`marketplace-prod-assets-in`, `marketplace-prod-public-in`, `tesseracthub-480811-mark8ly-pg-backups` if confirmed unused)
- Rotate or remove stale `default/tenant-router-service-secrets`
- Make `fga-init` Job idempotent (skip create if store named `mark8ly-platform` already exists) so future re-runs don't accumulate zombies
