# Go-live checklist

Companion to `GO-LIVE-PUNCHLIST.md`, which is the findings log. This is the
list you tick.

Every item carries **how to verify it**, not just what to do. That column is
the point of the document. The 2026-09-23/24 session found six mechanisms
that were configured, reviewed, believed correct, and doing nothing:

| Looked correct | Actually |
|---|---|
| Storefront key secret mounted, client sending it | server read a different env var; every route answered 200 to a wrong key |
| Drift counter moved into a scraped process | `CounterVec` with no children exports no series at all |
| Alert routing: a 200-line tree in git | rejected wholesale for 19 days over one missing secret |
| Reconciliation heartbeat registered | registered at zero, which reads as 1970, and paged a healthy pod |
| `readonly` allowlist naming seven recovery routes | matched none of them; an expired trial could not re-subscribe |
| Stripe key present in the pod | arrived with a trailing newline; every Stripe call failed at the header |

In each case the near-end check passed. **A mechanism is not wired until you
have seen the signal come out the far end.** For a gate that means a request
that *should* be rejected actually being rejected — not a 200 on the happy
path, which an absent gate also produces. For a metric it means the series in
Prometheus, not the code that increments it. For config it means the binary
reading it, not the variable existing in the pod.

---

## A. Blocks the first real merchant

| # | Item | Verify by | Status |
|---|---|---|---|
| A1 | Billing writes open | `POST /billing/subscription` returns something other than 503 | **flag on**, path never exercised |
| A2 | End-to-end billing run: subscribe → cancel → un-cancel → let a period roll | a real subscription in Stripe and matching `store_subscriptions` rows | **not done** |
| A3 | Merchant-facing billing copy read end to end | a human reads every string in the cancel and dunning flows | not done |
| A4 | Counsel sign-off; NZ decision | — | not done, longest pole |
| A5 | Storefront shared-secret gate enforcing | `curl` with a wrong `X-Storefront-Key` returns 404, not 200 | **done**, verified in prod |
| A6 | Expired merchants can recover | a `store_closed` tenant can reach `POST subscription/billing` | **done** (#919) |
| A7 | Stripe reachable | an API call succeeds; a store acquires a `stripe_customer_id` | **done** — first ever recorded 2026-09-24 03:24 |

A2 is the one that matters and the one that cannot be shortcut. It was
impossible before 2026-09-24: every Stripe call failed at the Authorization
header, so nothing could have been subscribed even by hand.

## B. Blocks a public launch

| # | Item | Verify by | Status |
|---|---|---|---|
| B1 | Funnel instrumentation | `grep -r '\.track(' apps/` returns hits | **zero repo-wide** |
| B2 | Sentry receiving events | an error appears in the Sentry UI | DSN injected, read by nothing |
| B3 | Log aggregation | a pod log line older than the ring buffer is still readable | not built |
| B4 | Alert delivery | a test alert arrives in `#falco-events` | routing **fixed** (#1081); delivery unproven |
| B5 | Backup restore test | a restore into a scratch cluster returns real rows | never done; 3-day retention |
| B6 | Replication supervised | a failover drill | async, unsupervised |
| B7 | Shopper privacy notice | the page exists and is linked | not written |
| B8 | Per-merchant analytics id | not one shared OpenPanel id | shared |

## C. Mobile (iOS) submission

Apple rejected under Guideline 2.1 — *information needed*, not a defect.

| # | Item | Verify by | Status |
|---|---|---|---|
| C1 | Reviewer can sign in | a fresh device signs in without an emailed code | **done** — `DEMO_LOGIN_EMAILS` covers the mobile path via `autologin.CompleteForProvider` |
| C2 | Demo account shows real content | the account has products and orders | **done** — `demo@mark8ly.com` owns The Bondi Store, 231 products |
| C3 | App Review Notes filled (items 2–7) | the field is non-empty in App Store Connect | not done |
| C4 | Screen recording on a physical device | attached to the submission | not done — only you can do this |
| C5 | Purpose strings give an example of use | read `NSCameraUsageDescription` against guideline 5.1.1 | present but thin |
| C6 | Universal links resolve | `/.well-known/apple-app-site-association` returns 200 | **404** — declared in `associatedDomains`, not served |
| C7 | Mobile tests run in CI | a red test blocks a build | **130 test files, zero run** |
| C8 | Crash reporting | a forced crash appears in a dashboard | none |

No in-app purchases exist (no StoreKit, no `react-native-iap`), so guideline
3.1.2 does not apply — say so in the Notes rather than leaving it inferred.

## D. Known-broken, shipping anyway

Recorded so nobody rediscovers them as surprises: Ireland silently dropped
from the country list, the second-store country gate, multi-store switching,
the unenforced 14-day tax window, and a CRM holding 259 leads with no
templates. See punchlist §3.

---

## Verification commands

Run against production. Each answers a question the dashboard cannot.

```bash
# A5 — the gate rejects a wrong key (404 expected, 200 means it is off)
#
# The wrong key goes through a variable rather than inline: gitleaks'
# curl-auth-header rule matches the SHAPE of an inline auth header, so a
# literal here fails the secret scan on every open PR in the repo, however
# obviously fake the value is.
WRONG_KEY=not-the-real-key
curl -s -o /dev/null -w '%{http_code}\n' \
  -H "X-Storefront-Key: $WRONG_KEY" \
  'https://api.mark8ly.com/api/v1/storefront/stores/demo-store/products?limit=1'

# B4 — warnings have a receiver at all (empty output means they reach "null")
kubectl get secret alertmanager-kps-kube-prometheus-stack-alertmanager-generated \
  -n monitoring -o jsonpath='{.data.alertmanager\.yaml\.gz}' \
  | base64 -d | gunzip | grep -A2 'severity: warning'

# Metrics exist rather than merely being incremented somewhere in Go
for m in mark8ly_subscription_reconciliation_drift_total \
         mark8ly_subscription_trial_reminders_sent_total \
         http:error_rate:ratio5m; do
  echo -n "$m "
  # expect a non-zero count; ABSENT means the series does not exist
done
```

## The deployment check that actually works

Matching an image tag answers the wrong question — a tag can be newer than
the fix and still predate it, which happened twice in one session. Ask
whether the running build **contains** the commit:

```bash
TAG=$(kubectl get pods -n mark8ly --field-selector=status.phase=Running \
  -o jsonpath='{range .items[*]}{.spec.containers[0].image}{"\n"}{end}' \
  | grep -oE 'marketplace-api:main-[a-f0-9]+' | head -1 | sed 's/.*main-//')
git merge-base --is-ancestor <commit> "$TAG" && echo DEPLOYED || echo "not yet"
```

Argo reporting `Synced` is not the same as the change being live. On
2026-09-24 it reported Synced against a revision that contained a fix while
the running Deployment still had the old value; a hard refresh corrected it.
