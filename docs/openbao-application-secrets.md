# Mark8ly application secrets

OpenBao is the default for all application credentials, including database,
provider, session, CI integration, and tenant credentials. GCP Secret Manager
retains only critical platform/bootstrap/recovery material, such as shared
registry authentication. Follow the infrastructure repository's
`docs/application-secret-policy.md`.

Static production values live at `kv/mark8ly/app/mark8ly-<secret-name>` in field
`value`; UAT uses `kv/mark8ly-uat/app/mark8ly-<secret-name>`. ESO readers have exact
read-only paths per namespace. Tenant credentials live under the owning tenant's
subtree and use the runtime client's base64 `value` format. Never mix these formats.

Mobile app credentials use the existing Kubernetes-authenticated OpenBao client
under `mark8ly/marketplace-api/tenants/<tenant>/app-credentials/mark8ly-<credential-type>`.
The logical `APPCREDS_PROJECT_ID` argument remains a compatibility namespace,
not an instruction to create resources in GCP. Invalid OpenBao configuration
fails startup; production has no in-memory fallback.

To sync a CI integration secret, authenticate `bao` with a short-lived session
that can read only the required source, then run:

```sh
python3 scripts/ops/sync-secret-openbao-to-github.py \
  mark8ly/app/mark8ly-sentry-auth-token SENTRY_AUTH_TOKEN
```

The script reads once, preserves bytes, and passes the value through stdin. It
never prints payloads or writes them to files. The GCP copy entrypoint is retired.

Internal-auth rotation requires a separately approved operation. Inventory all
current consumers; capture an encrypted recovery copy; grant a short-lived writer
only the exact path; write with CAS against the reviewed version; force ESO refresh
and verify actual byte equality before coordinating all consumer restarts through
GitOps. Verify authenticated inter-service calls and each rollout, then revoke
write access. A stale Ready condition alone is not proof of refresh. Never rotate
credentials as part of this storage migration. The old GCP rotation script now
fails closed instead of writing an obsolete source and restarting consumers.
