# Delhivery webhook — real-time tracking push

`marketplace-api` exposes a public webhook endpoint that Delhivery can
POST to on every scan (pickup, in-transit, out-for-delivery,
delivered). Configuring the webhook on a merchant's Delhivery account
collapses the tracking-refresh lag from 2 minutes (the background
poller's schedule) to a few seconds.

## Endpoint

```
POST https://<admin-host>/api/v1/carrier-webhooks/delhivery
```

Replace `<admin-host>` with the admin-panel hostname for the merchant
(example: `playwrite-test-admin.mark8ly.com`).

Authentication uses a shared secret that Delhivery sends back on every
request. We accept either form:

- `X-Webhook-Token: <secret>` header (preferred)
- `?token=<secret>` query-string parameter (fallback — use when the
  carrier's panel doesn't support custom headers)

The secret is the `secret_key` value stored on the store's Delhivery
carrier config row (admin → Settings → Shipping → Delhivery → Webhook
Secret). If left blank, the webhook receiver fails closed and all
calls return 401 — a non-empty secret must be configured before
pointing Delhivery at this URL.

## Configuring on one.delhivery.com

1. Log in to <https://one.delhivery.com>.
2. Open **Settings → Post Webhook**.
3. Paste the admin URL `https://<admin-host>/api/v1/carrier-webhooks/delhivery`.
4. Set the shared secret — use the same opaque string you saved in
   mark8ly admin → Settings → Shipping → Delhivery → Webhook Secret.
5. Save. Delhivery will start POSTing JSON on every scan.

## Payload shape

Delhivery POSTs a JSON body shaped like:

```json
{
  "Shipment": {
    "AWB": "1234567890",
    "Status": {
      "Status": "In Transit",
      "StatusType": "UD",
      "StatusDateTime": "2026-04-20T10:15:22"
    },
    "Scans": [
      {
        "ScanDetail": {
          "ScanType": "UD",
          "Instructions": "In Transit at Bengaluru Hub",
          "ScannedLocation": "Bengaluru DC",
          "ScanDateTime": "2026-04-20T10:15:22"
        }
      }
    ]
  }
}
```

`StatusType` values we map to our internal status vocab:

| Delhivery StatusType | mark8ly status |
| ---                  | ---            |
| `DL`                 | `delivered`    |
| `RT`, `UD`           | `exception`    |
| anything else        | `in_transit`   |

See `internal/shipping/delhivery.go` → `MapDelhiveryStatus`.

## Behaviour guarantees

- **Idempotent.** Delhivery retries on transient failures; the receiver
  uses the same advance path as the 2-minute poller, which is a no-op
  when the inbound status is not more advanced than the stored one.
- **Unknown AWBs return 200.** Prevents attackers from enumerating
  tracking numbers via timing / status-code differences.
- **Invalid token returns 401.** No DB mutation happens on failed
  auth.
- **Fast ack.** The HTTP 200 returns before the downstream side-effects
  (receipt email, in-app notification) complete, so Delhivery never
  sees a slow endpoint.

## Relationship to the background poller

The 2-minute `tracking-sync-cronjob` in `tesserix-k8s` is the
production safety net — it pulls Delhivery's tracking API for every
open shipment regardless of whether the webhook is configured. When a
merchant also configures the webhook, both paths feed the same
`order_events` timeline; duplicate events don't appear because the
underlying `AdvanceShipmentFromTracking` helper short-circuits when
the inbound status equals the stored status.

Disable the webhook path on a specific tenant by clearing the webhook
secret in admin settings — the poller will keep the timeline current.
