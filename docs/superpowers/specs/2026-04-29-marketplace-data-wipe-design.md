# Marketplace Data Wipe — Design

**Date:** 2026-04-29
**Status:** Design — pending approval
**Goal:** Wipe all test data from the Mark8ly marketplace prod environment so the platform is a clean v1 ready for real customers. Schema, disks, infra, secrets, and reference data preserved.

## Context

The user has been testing directly against the prod environment (`tesseract-prod-in-gke`, project `tesseracthub-480811`). Everything in the system today is test data: 4 tenants, 5 stores, 239 products, 102 orders, 188 GIP users, 12 OpenFGA tuples, 12.4 MB of merchant assets in GCS, ~36 docs in MongoDB. None of it is customer data; all of it must go.

The wipe must leave the platform in a state where:
1. A new merchant can sign up at `mark8ly.com` and complete onboarding end-to-end.
2. A customer can shop on a freshly-onboarded storefront.
3. No stale config, sessions, or orphan rows interfere with future onboarding.

## Inventory (what's being wiped)

### Postgres (`mark8ly-postgres` CNPG cluster, single instance)
| Database | Size | What's there |
|---|---|---|
| `mark8ly_marketplace_api` | 16 MB | 60+ tables — products (239), orders (102), order_items (103), order_events (327), variant_stock (274), product_variants (274), product_media (96), categories (90), payment_transactions (100), audit_logs (372), outbox_events (999), customers (10), customer_addresses (7), vendors (5), reviews (11), returns (11), shipments (19), pages (12), notifications (20), gift_cards (1), coupons (2), custom_domains (1, `primasyss.com`), campaigns (1), order_addresses (204), and 30+ zero-row tables |
| `mark8ly_platform_api` | 8.4 MB | tenants (4), stores (5), user_sessions (34), onboarding_sessions (8), verification_tokens (8), invitations (1), user_mfa (1), outbox_events (5). **Reference tables to preserve**: countries (20), states (117), currencies (16), timezones (22) |
| `mark8ly_openfga` | 7.7 MB | 6 stores (1 live `01KNP6EQYASKS3JPAEAWGYK5ZN` + 5 zombies), 12 tuples, 5 authorization_models, 12 changelog rows |

### MongoDB (`mark8ly-mongodb-0`, 0.36 MB used of 20Gi)
- `otto` database: conversations (6), messages (13), otto_audit (9), staff_availability (1)

### Redis (`redis-marketplace`, 16Gi PVC)
- Cached data only, no source of truth

### GIP (Google Identity Platform)
| Tenant | Users | Action |
|---|---|---|
| `MP-Customer-39opy` | 6 (incl. mahesh.sangawar@gmail.com) | Wipe users, keep tenant |
| `MP-Internal-e986p` | 180 (mostly synthetic test) | Wipe users, keep tenant |
| `Platform-9bu14` | 2 (incl. mahesh.sangawar@gmail.com) | Wipe users, keep tenant |
| `MP-Customer-zoe11` | 0 (zombie dupe) | Delete tenant |
| `MP-Internal-z5rnh` | 0 (zombie dupe) | Delete tenant |
| `Platform-2c9z0` | 0 (zombie dupe) | Delete tenant |

### GCS
- `tesseracthub-480811-mark8ly-media` — 12.4 MB merchant branding + product images for 3 tenants. Wipe contents, keep bucket.

### Cloudflare DNS
- 1 CNAME for `primasyss.com` (custom domain for `india store` tenant). Manual removal via Cloudflare dashboard.

### Pub/Sub
- 0 subscribers, 0 backlog → nothing to wipe. Topics stay.

## Constraints / Out of Scope

**Not touching** (explicitly):
- ArgoCD apps, ExternalSecrets, ConfigMaps, Secrets (all preserved)
- Knative, Istio, cert-manager, External-Secrets-Operator, CNPG operator
- Any namespace other than `mark8ly`, `marketplace`, `redis-marketplace`
- The `tesserix-*` Pub/Sub topics (shared with other platform apps)
- Cluster-wide Cloudflare secrets in `cert-manager`, `external-dns`, `cloudflared`
- The Workspace OAuth client (`849928263410-…`) — shared, not user data
- The 3 empty stale GCS buckets (`marketplace-prod-assets-in`, `marketplace-prod-public-in`, `tesseracthub-480811-mark8ly-pg-backups`)
- 781 stale CNPG `Backup` CRs in `mark8ly` ns
- 2 OutOfSync ArgoCD apps (`mark8ly`, `mark8ly-postgres`) — pre-existing drift
- Stale `default/tenant-router-service-secrets`

**Approach chosen:** Surgical TRUNCATE preserving schema, disks, and reference data. Rejected: drop+recreate DBs (overkill for 56 MB), nuclear PVC delete (brittle, ArgoCD churn).

**No backup taken.** User explicitly opted out — all data is test garbage.

## Procedure

### Section 1 — Pre-flight (quiesce)

Stop background writers so nothing writes during the wipe.

1. Suspend the every-2-min sync CronJob:
   ```
   kubectl patch cronjob mark8ly-marketplace-api-admin-tracking-sync -n mark8ly \
     --type merge -p '{"spec":{"suspend":true}}'
   ```
2. Suspend the CNPG ScheduledBackup:
   ```
   kubectl patch scheduledbackup mark8ly-postgres-scheduled-backup -n mark8ly \
     --type merge -p '{"spec":{"suspend":true}}'
   ```
3. Scale the 4 Go service deployments to zero:
   ```
   kubectl scale deployment -n mark8ly --replicas=0 \
     mark8ly-marketplace-api-admin \
     mark8ly-marketplace-api-storefront \
     mark8ly-platform-api \
     mark8ly-auth-bff
   ```
   (Next.js apps `admin`/`storefront`/`onboarding` stay up but will 5xx for the wipe window — acceptable, no users.)

### Section 2 — Postgres wipe

Per-DB pre-flight: dump the table list and verify the preserve list is exhaustive.

```sql
-- run against each DB before TRUNCATE
SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename;
```

For each DB, run a single atomic loop:

```sql
DO $$ DECLARE r RECORD;
BEGIN
  FOR r IN SELECT tablename FROM pg_tables
           WHERE schemaname = 'public'
             AND tablename NOT IN (<preserve_list>)
  LOOP
    EXECUTE format('TRUNCATE TABLE public.%I RESTART IDENTITY CASCADE', r.tablename);
  END LOOP;
END $$;
```

Preserve lists:
- `mark8ly_marketplace_api`: `'schema_migrations', 'schema_marker'`
- `mark8ly_platform_api`: `'schema_migrations', 'schema_marker', 'countries', 'states', 'currencies', 'timezones'` (verify via pg_tables before run)
- `mark8ly_openfga`: `'schema_migrations'`

Execution channel:
```
kubectl exec -n mark8ly mark8ly-postgres-2 -c postgres -- \
  psql -U postgres -d <db> -c "<sql>"
```

After the openfga TRUNCATE, the existing live store and authorization_model are gone. Re-run the init Job:

```
kubectl delete job -n mark8ly -l app=mark8ly-fga-init --ignore-not-found
# Re-apply via ArgoCD sync or kubectl apply on the manifest
```

Wait for the Job to complete and log `FGA_STORE_ID=…` for the new `mark8ly-platform` store. Apps will rediscover by name on next startup (`marketplace-api/internal/authz/client.go::DiscoverStoreID`, `platform-api/internal/authz/authz.go`).

### Section 3 — External systems wipe

In order (least → most permanent):

1. **Redis FLUSHALL**
   ```
   REDIS_PASS=$(kubectl get secret -n redis-marketplace redis -o jsonpath='{.data.password}' | base64 -d)
   kubectl exec -n redis-marketplace redis-0 -- redis-cli -a "$REDIS_PASS" FLUSHALL
   ```

2. **MongoDB drop `otto` DB**
   ```
   kubectl exec -n mark8ly mark8ly-mongodb-0 -- mongosh --quiet --eval \
     'db.getSiblingDB("otto").dropDatabase()'
   ```

3. **GCS wipe `mark8ly-media`** (keep bucket)
   ```
   gcloud storage rm -r 'gs://tesseracthub-480811-mark8ly-media/**' \
     --project=tesseracthub-480811
   ```

4. **GIP users — wipe in 3 active tenants** (keep tenants)
   For each tenant ID `T` in `MP-Customer-39opy`, `MP-Internal-e986p`, `Platform-9bu14`:
   - List uids: `gcloud identity-platform users list --tenant="$T" --format='value(uid)' --project=tesseracthub-480811`
   - Batch delete via REST `accounts:batchDelete`:
     ```
     curl -X POST \
       -H "Authorization: Bearer $(gcloud auth print-access-token)" \
       -H "Content-Type: application/json" \
       -d '{"localIds":[…uids…],"force":true}' \
       "https://identitytoolkit.googleapis.com/v1/projects/tesseracthub-480811/tenants/$T/accounts:batchDelete"
     ```
   - Verify post-delete: `users list --tenant="$T" --limit=1` returns empty.

5. **GIP zombie tenants — delete the 3 dupes**
   ```
   for T in MP-Customer-zoe11 MP-Internal-z5rnh Platform-2c9z0; do
     gcloud identity-platform tenants delete "$T" --project=tesseracthub-480811 --quiet
   done
   ```

6. **Cloudflare DNS** — manual: log into Cloudflare dashboard, find `primasyss.com` zone (or `mark8ly.com` subdomain delegation), delete the CNAME pointing to the storefront. The DB row for it is already gone from `mark8ly_marketplace_api.custom_domains` after Section 2.

### Section 4 — Verification + restart

Verify wipe before bringing apps back up:

```sql
-- per DB: every non-preserve table = 0 rows
SELECT relname, n_live_tup FROM pg_stat_user_tables
 WHERE n_live_tup > 0 ORDER BY relname;
```

```
kubectl exec -n mark8ly mark8ly-mongodb-0 -- mongosh --quiet --eval \
  'db.getSiblingDB("otto").getCollectionNames()'
# expect: []

kubectl exec -n redis-marketplace redis-0 -- redis-cli -a "$REDIS_PASS" DBSIZE
# expect: 0

gcloud storage ls gs://tesseracthub-480811-mark8ly-media/ --project=tesseracthub-480811
# expect: empty

# OpenFGA — exactly 1 store
curl -s http://mark8ly-openfga.mark8ly.svc.cluster.local:8080/stores | jq '.stores | length'
# expect: 1
```

Then bring apps back (reverse Section 1):

1. Resume `ScheduledBackup` and `tracking-sync` CronJob (`suspend: false`).
2. Scale all 4 deployments back to 1 replica.
3. `kubectl rollout status` on each.
4. `curl /health` and `/ready` on each pod.

### Stale-cookie risk

Existing browser sessions hold JWTs minted before the wipe. They reference deleted GIP UIDs. `auth-bff` validates UIDs against GIP on every request; deleted UIDs will return 401 → user redirected to login. **Verify this is the actual behavior** with a smoke test using a stale cookie. If it fails open instead of failing closed, mitigation is to rotate the `mark8ly-session` Secret which invalidates all cookies cluster-wide.

### Onboarding-must-work guarantee

After wipe, the following must hold for new merchant signup at `mark8ly.com`:

| Requirement | Preservation mechanism |
|---|---|
| GIP `MP-Internal-e986p` exists | Tenant kept (only users wiped) |
| GIP `MP-Customer-39opy` exists | Tenant kept |
| Workspace OAuth client wired to Google sign-in | Untouched |
| OpenFGA store named `mark8ly-platform` exists with model | Re-created by `fga-init` Job after wipe |
| Reference data (countries, states, currencies, timezones) | Preserved in `platform_api` preserve list |
| Slug uniqueness has no collisions | All tenants/stores wiped → any slug free |
| Email verification (SendGrid) | `mark8ly-sendgrid` Secret untouched |
| GCS media bucket exists | Bucket preserved (only objects wiped) |
| `mark8ly.com` onboarding URL routable | Cloudflare zone untouched (only `primasyss.com` CNAME goes) |

### Smoke test (post-wipe)

End-to-end proof onboarding works:

1. Open `mark8ly.com/sign-up` from a fresh incognito window.
2. Sign up with a real email → receive verification email → verify.
3. Complete onboarding wizard (store name, slug, country) → land on `<slug>-admin.mark8ly.com`.
4. Confirm DB rows: `tenants` (1), `stores` (1), `vendors` (1 if self-onboard creates one), `user_sessions` (1).
5. Confirm OpenFGA tuples: `tenant:<id>#owner@user:<uid>`, `store:<id>#owner@user:<uid>`.
6. Create a product, upload an image → image lands in GCS bucket, row in `products` table.
7. Visit `<slug>.mark8ly.com` storefront → product visible to anonymous users.

If 1–7 all pass, the platform is a clean v1 ready for real customers.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Preserve list misses a reference table → app breaks | Low | Medium | Pre-flight `SELECT tablename` step + visual confirm before TRUNCATE |
| Stale auth-bff cookie fails open instead of 401 | Low | Medium | Verify with stale-cookie smoke test; if needed, rotate `mark8ly-session` Secret |
| `fga-init` Job fails to recreate store → apps can't authorize | Low | High | Wait for Job Completed before scaling apps back; if it fails, debug script + re-run before restart |
| GIP `accounts:batchDelete` rate-limits with 180 users | Low | Low | Page in batches of 100 if needed |
| `tracking-sync` CronJob fires before suspend takes effect | Very Low | Low | Suspend is idempotent and immediate; worst case a few extra outbox rows that the Section 2 TRUNCATE catches anyway |
| ArgoCD self-heals our manual changes (e.g. unsuspends CronJob) | Medium | Low | Plan finishes within ~10 min; ArgoCD reconcile is every 3 min — if it un-suspends, we re-suspend. Acceptable thrash. |
| Cloudflare manual step forgotten | Medium | Low (DB row gone, just dangling DNS) | Explicit checklist item, not automated |

## Estimated wall-clock

- Pre-flight: ~1 min
- Postgres wipe (3 DBs): ~30s
- OpenFGA re-init: ~30s
- External systems wipe: ~3 min (most time is GIP user list+delete loops)
- Verification + restart: ~3 min
- Smoke test (manual): ~5 min

**Total: ~12-15 min** end-to-end.
