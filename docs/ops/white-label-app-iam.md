# White-Label Mobile App — IAM Requirements (spec §18.9)

This document is the Terraform handoff for the white-label mobile-app
add-on's credential IAM. The app-side choke-point lives in
`services/marketplace-api/internal/billing/appcreds/` — Terraform owns the
IAM bindings that back it.

## Secret Manager layout

Every merchant with the add-on purchased has four secrets named:

```
projects/{project}/secrets/merchant_{tenant_id}_apple-asc-api-key
projects/{project}/secrets/merchant_{tenant_id}_apple-asc-issuer-id
projects/{project}/secrets/merchant_{tenant_id}_apple-asc-key-id
projects/{project}/secrets/merchant_{tenant_id}_google-play-service-account
```

Secret names flatten the `/` separator in the logical §18.9 path to `_`
because Secret Manager disallows `/` in names. See `appcreds/paths.go`.

## Required IAM bindings

| Principal | Role | Scope | Rotation |
|---|---|---|---|
| `tesserix-ci@tesserix-prod.iam.gserviceaccount.com` | `roles/secretmanager.secretAccessor` | Condition: `resource.name.startsWith("projects/{project}/secrets/merchant_")` | Rotated via CI SA rotation cron (90d). |
| `gcp-eng-whitelabel@tesserix.com` (Google Group, ≤2 members) | `roles/secretmanager.secretAccessor` | Same condition. Audit-logged. | Group membership reviewed quarterly. |
| `marketplace-api-prod@tesserix-prod.iam.gserviceaccount.com` (workload SA) | `roles/secretmanager.secretVersionManager` | Condition: `resource.name.startsWith("projects/{project}/secrets/merchant_")` — can create + destroy versions, cannot read without also holding `secretAccessor` (it doesn't). | N/A — Workload Identity, no keys. |

**Deliberately NOT granted:**

- Project-level `roles/secretmanager.admin` to anything — including eng staff.
- `roles/secretmanager.secretAccessor` to `allAuthenticatedUsers` or any
  principal that doesn't match the `merchant_*` name-prefix condition.
- Any role to Developers (`gcp-eng@tesserix.com` broadly) — the choke-point
  is intentional.

## Audit logging

Enable Cloud Audit Logs at the project level for Secret Manager:

- `DATA_READ` — every `AccessSecretVersion`
- `DATA_WRITE` — every `CreateSecret`, `AddSecretVersion`, `DeleteSecret`
- `ADMIN_READ` / `ADMIN_WRITE` — already on by default

Sink the `DATA_READ` stream to BigQuery for the credential-access dashboard
alerts (P17 dashboards reference the Prometheus
`white_label_app_credential_accessed_total` counter emitted by the app —
the GCP audit log is the second line of defence if the app-side counter is
tampered with).

## App-side enforcement

`.github/workflows/ci.yml` rejects any PR that imports
`cloud.google.com/go/secretmanager` from a file outside
`services/marketplace-api/internal/billing/appcreds/`. See the
`appcreds-chokepoint` step in the `go` job.

## Terraform handoff

Implement as `tesserix-infra/terraform/03-secrets/whitelabel-app-iam.tf`.
This doc is the spec; pick up there for the actual `.tf` resources.
