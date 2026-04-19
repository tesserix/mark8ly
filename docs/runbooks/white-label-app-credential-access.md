# Runbook — White-Label Credential Access Spike

**Alert:** `WhiteLabelCredentialAccessSpike`
**Severity:** warning + `security: "true"`
**Owner:** security + platform
**Goes to:** `#security-alerts`

---

## What the alert says

`white_label_app:credential_accessed:rate1h` for some `type` has been
>3× its 24h median for 10 minutes sustained. This is §18.9 security
concern — Apple `.p8` or Google service-account JSON is being read
from Secret Manager at an unusual rate.

## Triage order

### 1. Identify the caller — audit log first

```bash
# Audit log is the source of truth: every appcreds.Load emits
# app_credential.read with actor + tenant + cred type.
kubectl exec -n mark8ly mark8ly-postgres-1 -c postgres -- \
  psql -U marketplace_user -d mark8ly_marketplace_api -c "
    SELECT
      metadata->>'actor'          AS actor,
      metadata->>'credential_type' AS cred_type,
      tenant_id,
      COUNT(*)                    AS reads
    FROM audit_events
    WHERE action = 'app_credential.read'
      AND created_at > now() - interval '1 hour'
    GROUP BY 1, 2, 3
    ORDER BY reads DESC
    LIMIT 20;"
```

### 2. Match actor pattern

| Actor prefix | Meaning |
|---|---|
| `user:<uuid>` | Merchant-driven — admin UI pulled creds for a build. Match the UUID to a user in `users` table. |
| `system:cron:day_90` | Day-90 teardown purge path — expected burst when multiple teardowns align. Cross-check with `white_label_app_state` for rows transitioning to `credentials_purged`. |
| `system:build-pipeline` | CI-driven build. Should only fire during a deploy; match with the CI run. |
| `system:ci` | Scheduled rebuild. Same as above. |
| Other / unknown | **INVESTIGATE NOW.** |

### 3. Expected burst scenarios (benign)

- **CI rebuild wave** — a platform-wide mobile-app CI run rebuilds N
  apps in parallel; each reads its creds once. N < 50 expected within
  a 1h window.
- **Day-90 purge batch** — the advancer calls `PurgeAll` which
  performs 4× delete per tenant. A cohort of 10 tenants → 40 delete
  events → 1h rate spike. Expected and visible in the lifecycle
  dashboard under "Day-90 completions".
- **Rotation drill** — an operator manually read creds to verify the
  GCP SM store is healthy. Recent incident doc should mention it.

### 4. Unknown actor or cross-tenant pattern

**Escalate immediately to security oncall.**

Additional queries:

```bash
# Is a single actor reading across multiple tenants?
kubectl exec -n mark8ly mark8ly-postgres-1 -c postgres -- \
  psql -U marketplace_user -d mark8ly_marketplace_api -c "
    SELECT
      metadata->>'actor'       AS actor,
      COUNT(DISTINCT tenant_id) AS distinct_tenants
    FROM audit_events
    WHERE action = 'app_credential.read'
      AND created_at > now() - interval '1 hour'
    GROUP BY 1
    HAVING COUNT(DISTINCT tenant_id) > 1
    ORDER BY distinct_tenants DESC;"
```

A single actor reading across >1 tenant in 1h is **unusual** — the
service role (`marketplace-api-prod`) does read across tenants during
purges, but user actors should only read their own tenant.

## GCP-side cross-check

The app-side audit log is one signal. GCP Cloud Audit Logs on Secret
Manager is the second line of defence (see `docs/ops/white-label-app-iam.md`):

```bash
gcloud logging read \
  'resource.type="secretmanager.googleapis.com/Secret"
   AND protoPayload.methodName="google.cloud.secretmanager.v1.SecretManagerService.AccessSecretVersion"
   AND protoPayload.resourceName=~"merchant_"' \
  --freshness=1h --project=tesserix-prod \
  --format='table(protoPayload.authenticationInfo.principalEmail,protoPayload.resourceName)'
```

If this returns **nothing** but the app-side counter shows a spike:
the app's Prometheus counter is lying (possible tampering). Page
security immediately.

If this returns the **same actors** as the audit log: the spike is
real and the actors are legitimate — proceed to "Acknowledge" below.

## Acknowledge

When confirmed benign:

1. Comment in the alert channel with the root cause (CI run / purge
   batch / operator action).
2. Silence the alert for the expected duration of the burst (1h
   default).
3. If this pattern recurs monthly (e.g. monthly CI rebuild), consider
   adjusting the alert multiplier from 3× to 4× — edit
   `tesserix-k8s/k8s/cluster/prometheus/rules/white_label_app.yaml`.

## Containment (if compromise suspected)

1. **Revoke the GCP principal**: `gcloud projects remove-iam-policy-binding`
   for any credential-accessor role on the suspected actor.
2. **Rotate all four credential types for affected tenant(s)** — the
   admin UI supports re-upload; old versions stay in SM but the
   `AccessLatest` path the app uses picks up new values on next call.
3. **File an incident** with audit log + GCP audit log attachments.

## References

- `services/marketplace-api/internal/billing/appcreds/` — chokepoint
- `docs/ops/white-label-app-iam.md` — IAM baseline
- Spec §18.9
- Alert rule:
  `tesserix-k8s/k8s/cluster/prometheus/rules/white_label_app.yaml`
