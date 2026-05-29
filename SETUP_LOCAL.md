# Local setup — mark8ly on `sandboxctl`

Bring mark8ly up on a local Kubernetes sandbox with
[`sandboxctl`](https://github.com/tesserix/sandboxctl) (kind + Argo CD + Istio +
in-cluster registry/Gitea, all on `https://*.sandbox.app:8443`).

> mark8ly is a multi-service monorepo, and **`sandboxctl` deploys one chart =
> one app = one URL**. So you deploy the shared **local-infra** datastores once,
> then each core service chart. `deploy` auto-applies `k8s/secrets.yaml` and
> auto-picks each chart's `values-local.yaml`.

> **Shared datastores.** mark8ly no longer ships its own Postgres. All local
> products share one **local-infra** release (CNPG Postgres + Redis + NATS +
> Mongo) with stable DNS, so deploy it FIRST. Each service `values-local.yaml`
> already points at those endpoints
> (`local-pg-rw.local-infra.svc.cluster.local:5432`,
> `mongodb.local-infra.svc.cluster.local:27017`, …), and the shared password
> arrives in the `mark8ly`/`marketplace` namespace via the reflected
> `local-infra-creds` Secret — products auto-connect with no extra wiring.

## 0. Prerequisites (one time)

```sh
command -v sandboxctl >/dev/null || brew install tesserix/tap/sandboxctl
```

The Helm charts live in the **tesserix-k8s** infra repo (a sibling checkout:
`../tesserix-k8s`). The shared datastores come from the `local-infra` chart;
each service has its own `values-local.yaml`.

## 1. Fill in local secrets

```sh
cp k8s/secrets.example.yaml k8s/secrets.yaml      # gitignored
$EDITOR k8s/secrets.yaml
```

`mark8ly-local-secrets` carries every key the core deployments reference (DB
passwords, GIP/OAuth, session/encryption/internal-auth, otto session). The
**“Prod source — GCP Secret Manager mapping”** table in
[`k8s/README.md`](k8s/README.md) shows where each value comes from in prod.

## 2. Bring the platform up (first run ≈ 10 min)

The shared Postgres uses CloudNativePG, so bring the cluster up `--with-cnpg`
(installs the CNPG operator):

```sh
sandboxctl up --with-cnpg --podman-disk 80 --podman-memory 12g
```

First `up` pulls a lot — **give it at least 10 minutes**. Later runs are fast.

### 2a. Deploy the shared datastores (local-infra) FIRST

This one release provides Postgres / Redis / NATS / Mongo for every product and
reflects `local-infra-creds` into the product namespaces. Deploy it before any
service:

```sh
sandboxctl deploy --chart ../tesserix-k8s/charts/apps/local-infra \
  --name local-infra --no-build
```

The mark8ly Postgres role (`mark8ly`) + DBs (`mark8ly_platform_api`,
`mark8ly_marketplace_api`, `mark8ly_openfga`), the Redis logical DB index, and
the `mark8ly_otto` Mongo DB are all provisioned by local-infra.

## 3. Credentials & status

```sh
sandboxctl creds      # Argo CD / Gitea / registry URLs + admin passwords
sandboxctl status
```

## 4. Deploy mark8ly

Run all of these from the **mark8ly repo root** (`cd /path/to/mark8ly`). The
shared datastores are already up from step 2a, so go straight to the services —
each builds from its own dir (`--repo`) and uses its chart. `values-local.yaml`
is auto-selected; URL = the `--name` you pass:

```sh
sandboxctl deploy --repo services/platform-api  --chart ../tesserix-k8s/charts/apps/mark8ly-platform-api               --name mark8ly-platform-api  --purge-old-tags
sandboxctl deploy --repo services/auth-bff       --chart ../tesserix-k8s/charts/apps/mark8ly-auth-bff                   --name mark8ly-auth-bff      --purge-old-tags
sandboxctl deploy --repo .                       --chart ../tesserix-k8s/charts/apps/mark8ly-marketplace-api-admin      --name mp-marketplace-admin  --purge-old-tags
sandboxctl deploy --repo .                       --chart ../tesserix-k8s/charts/apps/mark8ly-marketplace-api-storefront --name mp-marketplace-store  --purge-old-tags
sandboxctl deploy --repo .                       --chart ../tesserix-k8s/charts/apps/mark8ly-admin                      --name marketplace-admin     --purge-old-tags
sandboxctl deploy --repo .                       --chart ../tesserix-k8s/charts/apps/mark8ly-storefront                 --name mp-storefront         --purge-old-tags
sandboxctl deploy --repo services/otto           --chart ../tesserix-k8s/charts/apps/mark8ly-otto                       --name mark8ly-otto          --purge-old-tags
```

> **Frontends** (`admin`, `storefront`, marketplace-api UIs) build from the
> monorepo root (pnpm workspace) — that's why their `--repo` is `.`. The Go
> services build from their `services/<name>` dir.

URLs (once each shows `Synced + Healthy`):
`https://marketplace-admin.sandbox.app:8443`, `https://mp-storefront.sandbox.app:8443`, …

### Cleaner alternative — one `sandboxctl.yaml`

The above is one `deploy` per chart. For a true one-command bring-up, add a
[`sandboxctl.yaml`](https://github.com/tesserix/sandboxctl#multi-image-builds)
multi-image build manifest at the repo root declaring each service's
`dockerfile` + `context` + chart. Ask and I'll generate it.

## 5. Dependencies not covered by these charts

- **otto's MongoDB** is the shared `mongodb.local-infra.svc.cluster.local` from
  the local-infra release (step 2a) — its `values-local.yaml` already points
  `mongo.url` there (DB `mark8ly_otto`, root user, shared password). No
  per-namespace Mongo chart is needed.
- **OpenFGA** (`mark8ly-openfga`) is deferred; deploy it the same way as the core
  services if you need authorization checks. Its Postgres DB (`mark8ly_openfga`)
  is already provisioned on the shared local-infra Postgres.

## 6. Keep images & disk clean

```sh
podman system prune -a -f --volumes && podman builder prune -af
```

`--purge-old-tags` on each `deploy` also trims superseded tags from the
in-cluster registry.

## 7. Reset mark8ly's data (shared datastores)

The datastores are shared across products, so don't wipe everything to reset
mark8ly. Use the local-infra per-product cleanup, which drops + recreates only
mark8ly's Postgres DBs, flushes its Redis logical DB, and drops its Mongo DB —
other products are untouched:

```sh
../tesserix-k8s/charts/apps/local-infra/clean-product.sh mark8ly
```

## 8. Redeploy / tear down

```sh
sandboxctl deploy --repo services/platform-api --chart ../tesserix-k8s/charts/apps/mark8ly-platform-api --name mark8ly-platform-api --purge-old-tags   # after a change
sandboxctl undeploy --name mark8ly-platform-api    # remove one app
sandboxctl down                                    # wipe the whole sandbox
```
