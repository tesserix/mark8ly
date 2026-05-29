# mark8ly — local (kind) Secret

This directory holds the ONE Kubernetes Secret needed to run mark8ly on a
local / kind cluster. Production is unaffected: prod uses per-service
ExternalSecrets synced from GCP Secret Manager, defined in
`tesserix-k8s/external-secrets/prod/mark8ly/`. None of that is touched here.

## Quick start

```bash
cp k8s/secrets.example.yaml k8s/secrets.yaml      # copy-and-go
kubectl create namespace mark8ly --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f k8s/secrets.yaml
```

Deploy the shared `local-infra` datastores (CNPG Postgres + Redis + NATS +
Mongo) FIRST — it provisions mark8ly's Postgres role/DBs, Redis logical DB, and
the `mark8ly_otto` Mongo DB, and reflects `local-infra-creds` (same throwaway
password) into this namespace. Then install the service charts from
`tesserix-k8s` with their `values-local.yaml` overlays:

```bash
K=../tesserix-k8s/charts/apps

helm upgrade --install local-infra                       $K/local-infra                       -n local-infra --create-namespace -f $K/local-infra/values.yaml
helm upgrade --install mark8ly-platform-api             $K/mark8ly-platform-api             -n mark8ly -f $K/mark8ly-platform-api/values-local.yaml
helm upgrade --install mark8ly-marketplace-api-admin    $K/mark8ly-marketplace-api-admin    -n mark8ly -f $K/mark8ly-marketplace-api-admin/values-local.yaml
helm upgrade --install mark8ly-marketplace-api-storefront $K/mark8ly-marketplace-api-storefront -n mark8ly -f $K/mark8ly-marketplace-api-storefront/values-local.yaml
helm upgrade --install mark8ly-auth-bff                 $K/mark8ly-auth-bff                 -n mark8ly -f $K/mark8ly-auth-bff/values-local.yaml
helm upgrade --install mark8ly-admin                    $K/mark8ly-admin                    -n mark8ly -f $K/mark8ly-admin/values-local.yaml
helm upgrade --install mark8ly-storefront              $K/mark8ly-storefront              -n mark8ly -f $K/mark8ly-storefront/values-local.yaml
helm upgrade --install mark8ly-otto                     $K/mark8ly-otto                     -n mark8ly -f $K/mark8ly-otto/values-local.yaml   # uses shared mongodb.local-infra
```

Reach the UIs / APIs with `kubectl port-forward` (kind has no Istio gateway):

```bash
kubectl -n mark8ly port-forward svc/mark8ly-storefront 4203:4203
kubectl -n mark8ly port-forward svc/mark8ly-admin 4202:4202
kubectl -n mark8ly port-forward svc/mark8ly-platform-api 8086:8086
```

## The Secret: `mark8ly-local-secrets`

Every chart's `values-local.yaml` collapses all of its secret references onto
this single Secret. Keys:

| Key | Used by | Notes |
|-----|---------|-------|
| `POSTGRES_PASSWORD` | shared local-infra Postgres | `local-sandbox-dev` (matches the shared CNPG role + reflected `local-infra-creds`) |
| `username` / `password` | all Go services' DB env | builds `DATABASE_URL`; `username` MUST equal the shared CNPG role (`mark8ly`), `password` = `local-sandbox-dev` |
| `INTERNAL_AUTH_SECRET` | all services | shared `X-Internal-Auth` value |
| `SESSION_ENCRYPT_KEY` | marketplace-api, admin, storefront, auth-bff | session/handoff signing |
| `ENCRYPTION_KEY` | marketplace-api (admin+storefront) | field encryption (non-optional) |
| `STOREFRONT_KEY` | marketplace-api-storefront, storefront | storefront trust boundary |
| `CUSTOMER_SESSION_SECRET` | otto | customer session (non-optional) |
| `GIP_WEB_API_KEY` | auth-bff, admin, storefront | non-optional key; empty OK locally (GIP no-op) |
| `OAUTH_CLIENT_SECRET` | auth-bff | non-optional key; empty OK locally |
| `SENDGRID_API_KEY` | platform-api, marketplace-api, otto | optional; empty → stdout mailer |
| `AUDIT_INGEST_SECRET` | platform-api, marketplace-api, auth-bff | optional |
| `SENTRY_DSN` | marketplace-api | optional |
| `STRIPE_BILLING_SECRET_KEY` / `STRIPE_BILLING_WEBHOOK_SECRET` | marketplace-api-admin | optional |

Some keys (GIP/OAuth) are referenced by `secretKeyRef` **without**
`optional: true`, so they must EXIST even when empty — that is why the example
includes them with blank values.

## Building images for kind

All charts use `pullPolicy: IfNotPresent` with a `:local` tag, so build and
`kind load docker-image <name>:local` each service before installing. See each
`values-local.yaml` header for the exact build/load command.

> `k8s/secrets.yaml` is git-ignored — never commit real credentials.

## Prod source — GCP Secret Manager mapping

Locally these keys hold throwaway values. In prod, an ExternalSecret
(`tesserix-k8s/external-secrets/prod/mark8ly/externalsecret.yaml`) syncs each
from GCP Secret Manager (project `tesseracthub-480811`):

| Local key | GCP SM secret |
|-----------|---------------|
| GIP_WEB_API_KEY | prod-mark8ly-gip-web-api-key |
| GIP_WEB_API_KEY_RESOURCE_NAME | prod-mark8ly-gip-web-api-key-resource-name |
| OAUTH_CLIENT_SECRET | prod-mark8ly-oauth-client-secret |
| SESSION_ENCRYPT_KEY | prod-mark8ly-session-encrypt-key |
| ENCRYPTION_KEY | prod-mark8ly-marketplace-api-encryption-key |
| STOREFRONT_KEY | prod-mark8ly-marketplace-api-storefront-key |
| INTERNAL_AUTH_SECRET | prod-mark8ly-{marketplace,platform}-api-internal-auth |
| CUSTOMER_SESSION_SECRET | prod-mark8ly-otto-session-secret |
| AUDIT_INGEST_SECRET | prod-mark8ly-audit-ingest-secret |
| SENDGRID_API_KEY | prod-mark8ly-sendgrid-api-key |
| SENTRY_DSN | prod-mark8ly-sentry-dsn |
| STRIPE_BILLING_SECRET_KEY | prod-mark8ly-stripe-billing-secret-key |
| STRIPE_BILLING_WEBHOOK_SECRET | prod-mark8ly-stripe-billing-webhook-secret |
| platform-api db password | prod-mark8ly-platform-api-db-password |
| openfga db password | prod-mark8ly-openfga-db-password |
| marketplace-api db password | prod-mark8ly-marketplace-api-db-password |

Read a prod value: `gcloud secrets versions access latest --secret=<name> --project=tesseracthub-480811`
