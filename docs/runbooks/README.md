# Runbooks

Operational runbooks for marketplace-api alerts and common recovery
scenarios. Each alert that fires into PagerDuty / Slack has a
`runbook_url` annotation that links here.

## Index

### Subscription & billing (P1–P14)

No runbooks yet; file issues as alerts ship per plan.

### White-label app (P15)

- [White-label app lifecycle stuck](white-label-app-lifecycle.md) —
  `WhiteLabelLifecycleQuietFor48h`. Info-level. Triggers when the
  teardown cron hasn't advanced any state row in 48 hours.
- [White-label credential access spike](white-label-app-credential-access.md) —
  `WhiteLabelCredentialAccessSpike`. Warning + `security: "true"`.
  Fires when Secret Manager reads exceed 3× the 24h median for 10m.

### Database DR (P19)

- [CNPG primary failover](cnpg-primary-failover.md) — `CNPGSyncStandbyMissing`,
  `CNPGSyncStandbyLagHigh`, manual promotion flow. Covers RPO=0 / RTO≤2min
  at the 100-merchant tier.

## Conventions

Every runbook opens with:

- **Alert name** — matches the `alert:` field in the PrometheusRule
- **Severity**
- **Owner** — the on-call rotation that gets paged

Then:

1. Plain-English summary of what the alert measures.
2. First-5-minute triage commands (copy-pasteable).
3. Common benign causes with how to distinguish them.
4. Escalation path for the non-benign case.
5. Acknowledgment / silencing guidance.
6. References to code + spec sections.

## Authoring

New alert? Add the runbook in the same PR. The `runbook_url`
annotation in the alert rule MUST point to a real file on `main` by
merge time — a 404 when on-call clicks the link is worse than no link
at all.
