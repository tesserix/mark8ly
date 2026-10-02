# Mark8ly application secrets

Use OpenBao for application and tenant credentials. Prefix secret leaf names with
`mark8ly-`; isolate production, UAT, and tenant ownership. Kubernetes workloads
use scoped Kubernetes authentication and namespaced External Secrets stores.
Never grant an application a migration writer policy.

GCP Secret Manager is reserved for critical platform/bootstrap/recovery secrets.
Do not create new Mark8ly application credentials there or fall back to in-memory
credential storage in production. Legacy GCP adapters exist only for reviewed
migration compatibility. Keep payloads out of logs, files, source, and CLI args.

Follow `tesserix-k8s/docs/application-secret-policy.md` for migration, verification,
encrypted recovery capture, and retirement of old sources. Verify consumers before
deleting originals. Do not rotate credentials as part of a storage-only migration.
