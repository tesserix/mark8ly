# tesserix-k8s changes (applied in commit 63dd4c1)

Per-tenant carrier credentials (Delhivery, Razorpay, TaxJar, future
providers) moved from `shipping_carrier_configs.api_key_encrypted`
into **GCP Secret Manager** — one secret per
(tenant, domain, provider, field).

## What's applied

- `charts/apps/mark8ly-marketplace-api-admin/values.yaml`:
  `carrierSecretStore: "gcpsm"`, `gcpProjectId`,
  `carrierSecretPrefix: "mark8ly-test"`, `carrierIamBootstrap.enabled`.
- `charts/apps/mark8ly-marketplace-api-storefront/values.yaml`: same
  store-side settings (reads must match writes).
- `charts/apps/mark8ly-marketplace-api-admin/templates/deployment.yaml`:
  plumbs `SHIPPING_SECRET_STORE`, `GCP_PROJECT_ID`,
  `SECRET_NAME_PREFIX` into the container. Also plumbs (added by the
  mark8ly OpenBao migration; see `docs/superpowers/specs/2026-09-03-openbao-carrier-secrets-design.md`):
  `OPENBAO_ADDR` (OpenBao API address), `OPENBAO_ROLE` (Kubernetes auth
  role), `OPENBAO_KV_MOUNT` (KV v2 mount, must be `"kv"`). marketplace-api
  now requires all three whenever `SHIPPING_SECRET_STORE` is anything
  other than `"inline"` — including `"gcpsm"` — because `ChainStore`
  routes an already-migrated `bao://` reference to OpenBao by prefix
  regardless of which mode is configured. Do not unset these on a
  `bao` -> `gcpsm` rollback.
- `charts/apps/mark8ly-marketplace-api-admin/templates/carrier-iam-bootstrap.yaml`:
  Helm post-install/post-upgrade Job that idempotently grants
  `roles/secretmanager.admin` to the marketplace-api GCP SA. Replaces
  every previous manual `gcloud` step — operators never touch IAM.

## IAM model

The binding is **unconditional**. GCP IAM cannot gate
`secretmanager.secrets.create` by `resource.name.startsWith(...)`
because at create-time `resource.name` evaluates to the parent
project, not the future secret. See:
<https://cloud.google.com/iam/docs/conditions-attribute-reference#resource-name-binary-prefix>.

The security boundary is therefore:

1. A dedicated SA
   (`app-secrets-marketplace-prod@tesseracthub-480811.iam.gserviceaccount.com`)
   bound only to marketplace-api pods via Workload Identity.
2. Application-layer naming: Go code only ever writes secrets under
   the `mark8ly-{env}-*` prefix (enforced in
   `internal/carriersecrets/refs.go`).

## Rollout order

1. Merge + deploy this chart. ArgoCD auto-syncs.
2. Helm post-install Job applies the IAM binding (idempotent on
   re-install / re-upgrade).
3. marketplace-api pods come up, log
   `carriersecrets: hybrid store online`.
4. Every new carrier-config save creates a GCP SM secret. Every read
   of a legacy `noop:` / `aes:` row lazily rewraps to `gsm://`.
5. No data-migration job required.

## Rollback

Flip `carrierSecretStore: "inline"` in the values file and resync.
The app falls back to the old envelope-encrypted DB column path.
`gsm://` rows resolve via the HybridStore's Get path which still
works in inline mode (it just doesn't rewrite). Rolling forward
again is a no-op.
