# Patches for tesserix-k8s (apply separately)

This worktree moves per-tenant carrier credentials (Delhivery, Razorpay,
TaxJar, future providers) from the `shipping_carrier_configs.api_key_encrypted`
Postgres column into GCP Secret Manager. The application code is complete
and tested; the infrastructure knobs still need to land in
`tesserix-k8s`. The changes below are the minimum needed to turn on
`SHIPPING_SECRET_STORE=gcpsm` in the test cluster (and later prod).

Everything below is **to be applied by hand in the `tesserix-k8s` repo**
— do not apply manifests with `kubectl apply`. The team's rule is
ArgoCD-only.

---

## 1. `charts/apps/marketplace-api/values.yaml`

Add the following under the existing `env:` map (next to `ENCRYPTION_MODE`):

```yaml
env:
  # ... existing keys ...

  # Per-tenant carrier credential store. "gcpsm" reads/writes one
  # GCP Secret Manager secret per (tenant, domain, provider, field).
  # "inline" falls back to envelope-encrypted DB columns.
  SHIPPING_SECRET_STORE: "gcpsm"

  # GCP project hosting the carrier secrets. Must match the project
  # the workload-identity SA (app-secrets-marketplace-prod@...) has
  # Secret Manager Admin on.
  GCP_PROJECT_ID: "tesseracthub-480811"

  # Prefix on every secret ID. Scopes IAM bindings via
  # resource.name.startsWith(...) so the prod SA cannot touch test
  # secrets and vice-versa.
  SECRET_NAME_PREFIX: "mark8ly-prod"
```

For the `test` overlay (`values-test.yaml` or equivalent), override:

```yaml
env:
  SHIPPING_SECRET_STORE: "gcpsm"
  GCP_PROJECT_ID: "tesseracthub-480811"
  SECRET_NAME_PREFIX: "mark8ly-test"
```

For dev-loop bringup where GCP Workload Identity isn't wired yet,
leave `SHIPPING_SECRET_STORE: "inline"` — the app will boot and
behave exactly like today.

---

## 2. IAM binding — `app-secrets-marketplace-prod@tesseracthub-480811.iam.gserviceaccount.com`

The workload-identity SA needs `roles/secretmanager.admin` scoped by
IAM condition so it can only CRUD secrets whose name starts with the
expected prefix. Two bindings — one per cluster / namespace prefix —
so prod can never touch test and vice-versa.

The binding lives in `tesserix-k8s/gcp-iam/marketplace-api.yaml` (or
whichever module owns marketplace-api's SA bindings). Add:

```yaml
# Prod binding — scoped to mark8ly-prod-* secrets.
- member: "serviceAccount:app-secrets-marketplace-prod@tesseracthub-480811.iam.gserviceaccount.com"
  role: "roles/secretmanager.admin"
  condition:
    title: "mark8ly-prod carrier secrets only"
    description: "Restrict prod SA to mark8ly-prod-* Secret Manager secrets"
    expression: |
      resource.name.startsWith("projects/_/secrets/mark8ly-prod-")

# Test binding — scoped to mark8ly-test-* secrets (same SA, separate
# condition so the test namespace workload can CRUD its own bucket).
- member: "serviceAccount:app-secrets-marketplace-prod@tesseracthub-480811.iam.gserviceaccount.com"
  role: "roles/secretmanager.admin"
  condition:
    title: "mark8ly-test carrier secrets only"
    description: "Restrict test SA to mark8ly-test-* Secret Manager secrets"
    expression: |
      resource.name.startsWith("projects/_/secrets/mark8ly-test-")
```

If marketplace-api test and prod use *different* Google service
accounts, split these into two separate members with only the
relevant prefix. Don't widen past `roles/secretmanager.admin` — the
app needs Create + AddVersion + Access + Delete, which this role
covers and nothing more.

---

## 3. Workload Identity binding (only if not already present)

The Kubernetes SA that marketplace-api runs as must be
workload-identity-bound to the Google SA above. If this is the first
time marketplace-api needs GCP SM write access, also add:

```yaml
- member: "serviceAccount:tesseracthub-480811.svc.id.goog[marketplace/marketplace-api]"
  role: "roles/iam.workloadIdentityUser"
  resource: "projects/tesseracthub-480811/serviceAccounts/app-secrets-marketplace-prod@tesseracthub-480811.iam.gserviceaccount.com"
```

`make dev` / local runs don't need this — they fall back to
`SHIPPING_SECRET_STORE=inline`.

---

## 4. Roll-out sequence

1. Merge and deploy the tesserix-k8s change **with `SHIPPING_SECRET_STORE=inline` left on** and only the IAM binding added. This gives the SA write permission without changing app behaviour.
2. Flip `SHIPPING_SECRET_STORE=gcpsm` in the values file and sync. The app will read existing `noop:`/`aes:` rows via the legacy fallback, and **will rewrite them to `gsm://` references on the next read** (lazy migration via `HybridStore.MaybeRewrap`). No downtime; no data migration job needed.
3. After a full week with no errors, drop the fallback `ENCRYPTION_KEY` from the deployment — it becomes purely used by the inline dev mode. All carrier credential read paths (admin shipments, storefront shipping rates, checkout_ext, payment_methods, webhooks) go through `carriersecrets.Store` and lazily rewrap any remaining legacy rows to `gsm://` on read.

---

## 5. Rollback

Set `SHIPPING_SECRET_STORE=inline` and redeploy. Any rows already
migrated to `gsm://` references will fail to decrypt (InlineStore
rejects gsm:// references), so a rollback after full migration
requires:

- Flip `SHIPPING_SECRET_STORE=gcpsm` back on so reads work.
- Implement the reverse rewrap (not shipped here — we deliberately
  don't expose it).

Practically: don't roll back once every row is migrated. The safe
roll-back window is the first week, where most rows are still
inline.

---

## 6. Nothing to do in `db-schema-bootstrap`

Schema is unchanged: `shipping_carrier_configs.api_key_encrypted` is
still a `text` column. We just persist a different flavour of opaque
string in it. No migration SQL needed.
