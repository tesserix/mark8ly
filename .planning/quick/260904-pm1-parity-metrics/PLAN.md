---
id: 260904-pm1
slug: parity-metrics
date: 2026-09-04
issue: tesserix-home#328 (gap 3)
kind: quick
---

# Make a catalog parity difference something that is noticed

## Gap 3 as filed, and why the answer is not a table

tesserix-home#328's third gap says:

> *"The evidence trail is logs, not a table. The monitor calls `slog` and persists
> nothing … Judging 'durably zero' will mean a log query over a window, and pod
> restarts do not reset the underlying data but do reset what `kubectl logs` can show."*

That was written while "durably zero" was a **cutover gate** — a decision someone was
going to make once, by looking. The cutover shipped (#631, #632), so the question the
table was for is answered and will not be asked again.

The ongoing need is different, and measuring it changes the answer:

| | state |
|---|---|
| `consolecatalog` exports a Prometheus metric | **no** — the package imports only `log/slog` |
| any alert rule mentions catalog or pricing | **no** — `tesserix-k8s/k8s/cluster/prometheus/rules/` has cnpg, subscription, white_label_app, and nothing else |
| evidence is durable | yes — Cloud Logging retains it; only `kubectl logs` is truncated by restarts |

So the real defect is not that the evidence is hard to query. **It is that a parity
difference in production would be written to a log that nothing watches.** A table
would not fix that: a table also has to be looked at, and nobody is looking.

What is needed is for a difference to *arrive somewhere*. That is a metric and an
alert, and the repo already has the pattern — `internal/campaignbudget/metrics.go`
declares metrics and `main.go:962` registers them with
`prometheus.DefaultRegisterer`.

## Why this matters more since #648

Before, `catalog.go` was hand-maintained and the parity check caught a human forgetting
to update it. Since #648 the fallback is **generated from the console**, so a
difference now means something narrower and more surprising: the console moved and
nobody regenerated, or someone hand-edited a generated file. Both are exactly the
kind of thing that goes unnoticed for weeks — the catalog keeps working, because the
console is authoritative and the fallback is only consulted when the console is
unreachable. **The failure is silent until the day it is load-bearing.**

## The distinction that must survive into the metrics

`Result.Compared` is separate from `Result.Differences` for a reason the package states:

> *"A failed read must not look like agreement: reporting zero differences when the
> console could not be reached would make an outage indistinguishable from a clean run."*

A naive single gauge re-creates that defect precisely. If the monitor stops being able
to reach the console, `differences` stays at its last value — or worse, is never set
and reads as absent-or-zero — and the alert stays quiet forever. **A monitor that has
stopped checking must not look like a monitor that keeps finding nothing.**

So the metrics must make "we compared and found nothing" distinguishable from "we did
not compare", and the alerting must fire on the second as well as the first.

## Tasks

### T1 — Metrics on the parity monitor

`internal/billing/consolecatalog/metrics.go`, following `campaignbudget/metrics.go`'s
shape, registered from `cmd/marketplace-api/main.go` beside the existing
`campaignbudget.MustRegisterMetrics`.

At minimum the monitor must express:

- how many differences the **last completed comparison** found
- when a comparison last **completed successfully** — the staleness signal, and the
  half that keeps a dead monitor from reading as a clean one
- that a comparison **failed** (console unreachable, bad read), counted separately
  from finding differences

Do not add a metric whose only reading is "0" in every state that matters; each one
must have a state where it is the thing that tells you.

`Monitor.Check`/`report` currently take a `*slog.Logger` and nothing else. Keep logging
exactly as it is — the log line is still the human-readable evidence and #328's
"clean runs log at info so the evidence trail is visible" reasoning still holds. The
metrics are added beside it, not instead.

### T2 — The alert rule

A `PrometheusRule` in `tesserix-k8s/k8s/cluster/prometheus/rules/`, added to that
directory's `kustomization.yaml`, following `subscription.yaml`'s shape (it has a
`subscription_test.yaml` beside it — check whether that is a promtool unit test and
match the convention if so).

Two alerts, not one:

1. **Differences found** — the catalog and the fallback disagree.
2. **No successful comparison in N hours** — the monitor is not running, or cannot
   reach the console. Without this, silence is ambiguous and the first alert is
   unreliable by construction.

Pick N from the real interval: the monitor runs every 15m in production
(`consolecatalog: parallel run enabled interval=15m0s`), so N should tolerate a
restart and a deploy without crying wolf. Say in the rule's annotation what an
operator should DO, not just what happened — a difference means regenerate
`catalog_data.go` from the console (`gencatalog -source=console`) and work out why it
drifted.

## Out of scope

- A persisted table. Argued above.
- Changing the comparison itself, the cache, or anything on the serving path.
- Console-side parity (`plan_catalog_parity_runs`) — a different comparison, already
  queryable.

## Verification

```
cd services/marketplace-api && go build ./... && go test -race -count=1 ./...
```

For the rule, whatever `promtool` invocation the existing rules are checked with — find
it before inventing one.
