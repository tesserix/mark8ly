# Image-to-3D generation (Meshy and friends)

> **Status: DEFERRED IN FULL as of 2026-10-01.** Not scheduled, not
> partially started, nothing depends on it. The figurine flow ships
> without any of this: the merchant opens the order, downloads the
> buyer's photograph at full resolution, and processes it with whatever
> tools they already use.
>
> Kept because the analysis holds and the decisions below were real
> ones. When it is picked up, enter at **phase 3 (case (c) — merchant
> turns their own product photographs into 3D for the catalog)**; phases
> 0–2 are its prerequisites and phase 0 may need redoing, since provider
> pricing, APIs and retention terms in this market change faster than
> this document will.
>
> What deferring removes from the near-term build, for the record: the
> `internal/model3d` package, a poller/worker and its dispatcher,
> provider due diligence and a DPA, a sub-processor disclosure in every
> merchant's privacy policy, usage metering that `internal/billing` does
> not have, a `plangate` feature across every plan row, a GLB viewer or
> turntable pipeline in a storefront with eleven runtime dependencies,
> and the unresolved question of whether a provider that retains inputs
> for training can satisfy a GDPR erasure request. That last one was the
> scariest open item in the estate's privacy story and it is now simply
> absent.
>
> Depends on every phase of
> [`2026-10-01-custom-products-design.md`](./2026-10-01-custom-products-design.md)
> — there is nothing to generate from until buyer uploads exist.
>
> API specifics below were written from knowledge that predates the
> current Meshy release. Every endpoint, parameter and turnaround figure
> needs checking against their live docs before phase 1 starts. The
> architecture does not depend on which of them are still accurate; the
> estimates do.

## Decisions taken (2026-10-01)

0. **The whole integration is deferred.** Decisions 1–3 stand as the
   shape it takes *when* it happens; none of them is being built now.
1. **Case (c) is the first and only slice to design against** — the
   merchant turns their own product photographs into 3D for the catalog.
2. **Buyer-sourced generation happens only after payment succeeds.**
   Pre-purchase buyer preview — case (a) below — is **dropped**, not
   deferred. Nothing in the storefront or either mobile app ever
   triggers a generation.
3. **Platform API key with a per-plan quota, plus BYO merchant key in
   OpenBao** as the escape hatch for anyone who outgrows it. Both, not
   one.

The analysis that produced those decisions is kept below because the
reasoning constrains the implementation, not only the roadmap.

## The question that was answered before writing any code

"Connect to Meshy" is three different systems depending on what the mesh
is *for*, and they do not share a cost model, a quality bar or a legal
surface:

**(a) Buyer-facing preview.** The shopper uploads a photo, sees a 3D
figurine spin, and buys. The mesh is a sales asset.

**(b) Merchant production input.** After the order, the merchant gets a
mesh as the starting point for sculpting or printing. The mesh is a tool.

**(c) Merchant catalog authoring.** The merchant turns their own product
photographs into 3D models for the product page. No buyer involvement.

**Decided: (c), then (b). (a) is dropped.**

Not because it is harder — the pipeline is the same. Because of what it
promises. A single-photo image-to-3D result is a plausible guess, not a
rendering of the thing the merchant will hand-finish and ship. If a
buyer approves a spinning mesh and receives something recognisably
different, that is "materially not as described": a chargeback the
merchant loses, under consumer-law regimes that do not care that an AI
produced the picture. (c) has no buyer, no promise and a cost ceiling
set by the merchant's own catalog. (b) has a cost incurred only after
money is taken. (a) is the one that can generate a legal obligation out
of a $0.20 API call.

Dropping (a) is worth more than it looks. It removes the storefront
polling UI, the buyer-facing wait, the "AI impression, not a proof"
disclaimer, the abuse vector where browsing burns the budget, and — see
below — most of the App Store exposure. The storefront's only
involvement becomes displaying meshes the merchant published, which is
case (c) and has no AI in it at the point of sale.

## The cost problem, which is the real design constraint

Every generation costs money per call. `internal/billing` has **no usage
metering of any kind** — it meters subscriptions, add-ons and
proration, and nothing counts events. So:

**Pre-purchase generation makes cost of goods a function of browsing
rather than of sales.** Ten thousand shoppers uploading selfies to watch
them spin is ten thousand paid calls and zero orders. Rate limiting
bounds the slope, not the total.

Three consequences, all load-bearing:

1. **Buyer-sourced generation runs after payment succeeds** — decided.
   The seam is `order.Service.Confirm`
   (`internal/order/service.go:224`), which takes the
   `payment_status → paid` transition and appends
   `EventKindPaymentRecorded` inside one transaction. Insert the
   `queued` generation rows **in that same transaction**: "this order is
   paid" and "its meshes are queued" then commit or fail together, and
   the dispatcher picks them up by polling. No outbox hop is needed —
   there is no external system to notify at that instant, only a row for
   our own worker.

   Cost then never exceeds a fixed fraction of revenue, and no shopper
   ever waits on a provider — image-to-3D is minutes, and nothing in a
   checkout can block on minutes.
2. **Cache on the content hash.** `media.BuildStorageKey` is already
   sha256 content-addressed, so a generation cache keyed on
   `(content_hash, provider, model_version, params_hash)` falls out for
   free. The same photo submitted twice is paid for once. For (c), where
   a merchant will re-run the same product shot while tuning, this is a
   large fraction of the bill.
3. **A per-store monthly budget, enforced before the call.** A new
   `plangate` feature `model3d_generations_per_month`, which means
   touching every plan row (`matrix_test.go` enforces that).

   **The quota blocks case (c) and never blocks case (b).** A merchant
   generating from their own catalog is told "you have used this
   month's generations" and can wait or upgrade — no one is harmed. A
   *paid order* is different: the buyer's money is already taken, the
   merchant owes them an item, and refusing to produce the merchant's
   production asset because of a billing ceiling is the platform
   failing at the only moment that matters. So (b) always proceeds,
   counts against `model3d_store_usage` as overage, and surfaces as a
   prompt to upgrade — never as a blocked job. This asymmetry needs a
   test of its own; it is the kind of rule a later refactor "tidies
   away".

Whose key pays is a commercial decision, not a technical one:

| | Platform key, plan quota | Merchant's own key in OpenBao |
|---|---|---|
| Merchant setup | none | sign up for Meshy, paste a key |
| Who eats a bad month | us | them |
| Fits the maker pitch | yes | no |
| Precedent in repo | — | `internal/carriersecrets`, `internal/bao` |

Decided: **both.** Platform key with a plan quota is the default,
because the maker this feature courts will not go and get an API key,
and "it just works on Starter" is a differentiator a competitor cannot
copy in a sprint. A store may override with its own key, held in
OpenBao exactly as carrier credentials are
(`internal/carriersecrets`), and when it does, its generations bypass
the quota entirely — it is their bill.
`model3d_generations.paid_by_merchant_key` records which key paid,
because the usage counter must not count spend that was never ours.

## Product decisions

**1. The provider is never named in the domain.** A new package
`internal/model3d` owns:

```go
type Generator interface {
    Submit(ctx context.Context, req SubmitRequest) (providerJobID string, err error)
    Poll(ctx context.Context, providerJobID string) (Result, error)
}
```

with a `MeshyGenerator` adapter and a `FakeGenerator` for tests —
exactly the shape of `media.Uploader` / `media.FakeUploader`, and the
reason that package's tests never touch GCS. Tripo, Rodin, Kaedim and
whatever ships next quarter then swap at the wiring in `main.go`. This
market reprices and re-APIs itself every few months; the seam is cheap
insurance, and it is also the only way the integration tests stay
hermetic.

**2. Poll, do not depend on a callback.** A provider webhook is an
optimisation, not the mechanism. If Meshy offers one, mount it at
`/render-webhooks/meshy` — the convention
`internal/handlers/public/routes.go:43` establishes for exactly this
("putting shipping webhooks on a separate root keeps both clean") — and
treat it purely as a hint to poll early. The poller stays the source of
truth, because a missed callback that nothing reconciles is an order
stuck forever, and the estate's recurring failure mode is precisely the
integration that looks configured and quietly emits nothing.

**3. Copy the asset into our bucket on completion.** Providers return
time-limited download URLs for the GLB and its textures. Storing one is
storing a 404 with a delay fuse. On completion the worker downloads and
re-uploads into the private bucket from the custom-products design, then
records our key. Nothing outside `internal/model3d` ever holds a
provider URL.

**4. Two quality passes, and the second is opt-in.** Meshy's shape has
historically been a fast coarse preview followed by a paid refine. For
(c) the merchant previews, then clicks refine on the one they like. For
(b) refine runs unattended because there is no one to ask. Do not refine
everything by default — it is the larger share of the bill.

**5. Generated meshes are drafts, and the UI says so.** For (c) the
merchant explicitly publishes a mesh to the product before any shopper
sees it. There is no path from "provider returned a file" to "live on
the storefront" without a human.

## Non-goals

- **Buyer-facing pre-purchase generation (case (a)).** Dropped by
  decision, not pending. A buyer never triggers a generation and never
  waits for one.
- Text-to-3D. Different product, no buyer photo, no reason yet.
- Rigging, animation, retopology, UV work.
- **Print-readiness guarantees.** Single-image output is routinely
  non-manifold, hollow in the wrong places and unscaled. The merchant's
  slicer or sculptor is the human in that loop, and the UI must not
  imply otherwise.
- AR quick-look (USDZ) on the storefront. Worth wanting; later.
- Letting buyers iterate ("try again, make the nose smaller"). Each
  retry is a paid call and an invitation to burn the budget.

## Data model

```
model3d_generations
  id              uuid pk
  tenant_id       uuid not null
  store_id        uuid not null
  -- exactly one source, enforced by CHECK:
  product_media_id              uuid    -- (c) merchant catalog authoring
  order_item_personalisation_id uuid    -- (b) post-payment production asset
  paid_by_merchant_key          boolean not null default false
  source_content_hash varchar(64) not null   -- the cache key
  provider        varchar(40) not null       -- 'meshy'
  provider_job_id varchar(200)
  model_version   varchar(60) not null
  params_hash     varchar(64) not null
  stage           varchar(20) not null       -- preview|refine
  status          varchar(20) not null       -- queued|submitted|running|completed|failed|cancelled
  attempts        int not null default 0
  heartbeat_at    timestamptz
  glb_storage_key text                       -- ours, never the provider's URL
  thumb_storage_key text
  poly_count      int
  error_code      varchar(60)                -- provider's, normalised
  error_detail    text                       -- sanitised; see providerlog
  cost_units      numeric(12,4)              -- what the provider charged
  submitted_at, completed_at, created_at, updated_at
  UNIQUE (store_id, source_content_hash, provider, model_version, params_hash, stage)
  INDEX (status, heartbeat_at)

model3d_store_usage          -- the budget counter
  store_id, period_month (date), generations int, cost_units numeric(12,4)
  PRIMARY KEY (store_id, period_month)
```

The `UNIQUE` is the cache: a second request for the same image and
parameters finds the completed row instead of paying again.

Status vocabulary and the worker deliberately copy `internal/csvjob`,
which already solved this exact problem in this exact codebase —
`ClaimJob` moving one job to `running` for exactly one caller,
`FOR UPDATE SKIP LOCKED`, heartbeats, `RecoverOrphanedJobs` for the pod
that dies mid-job, and a `Dispatcher` started in-process from `main.go`
rather than as a separate binary. Read the comment at
`internal/csvjob/dispatcher.go:17` before designing anything here: it is
a note about the queue that shipped with no consumer, so every upload
sat at `queued` forever (#897). The equivalent bug here is an order that
never gets its mesh and nothing that notices.

One difference from `csvjob`: this worker is I/O-bound on someone else's
minutes, so it is a poller with backoff, not a processor. Submitted jobs
are polled on a schedule with a cap on total wall-clock (say 30 minutes)
after which the job fails, the merchant is told, and no retry happens
automatically — an automatic retry on a provider timeout is how you pay
twice for one mesh.

## Privacy, consent and the provider as sub-processor

Buyers will upload photographs of their children, their partners, their
dead pets, other people's children, and Mickey Mouse. Sending those to a
third party is a processor relationship with teeth:

- A DPA with the provider, and the provider listed as a **sub-processor**
  in the storefront privacy policy — which is per-merchant copy
  (`docs/marketing/seeds/bondi-policy-pages.sql` shows how those pages
  are seeded), so the merchant's own policy has to say it too. A
  merchant who did not know their shoppers' photos leave the platform
  has a problem we gave them.
- An explicit buyer-facing statement at the upload control for case (a):
  this image is processed by a third-party AI service. Not buried.
- The provider's input-rights terms, which typically push the warranty
  onto whoever submits. That is the merchant, via us.
- **Erasure has a second copy to chase.** The custom-products design
  already adds blob deletion to `internal/customererasure` — this adds
  the generated GLB *and* a call to the provider's delete endpoint. If
  the provider has no delete API, or retains inputs for model training,
  that is a fact to establish in phase 0 and disclose, not discover
  during a request. It may be the thing that decides the provider.
- Retention: provider-side inputs should be deleted as soon as the asset
  is copied into our bucket, not at erasure time.

**Apple, specifically — much smaller now.** Dropping case (a) means
neither mobile app ever generates anything: `apps/mobile-storefront`
shows published meshes, `apps/mobile-admin` views a merchant's
generations read-only, and every `Submit` call originates from the web
admin. That takes the generative-AI questionnaire and the
UGC-moderation line of enquiry off the table for both submissions.
Still worth saying in the review notes that the merchant-facing web
console uses a third-party AI service, and still worth not landing this
in the same release as a pending review.

## The viewer

Nothing in this monorepo depends on `three`, `@react-three/*` or
`model-viewer` today; the storefront's runtime dependencies are eleven
packages. A GLB viewer is the heaviest thing anyone would have added to
it.

- **Admin** is where it should land first, and the bundle cost there is
  nearly free — a merchant inspecting a generated mesh is already in a
  heavy authoring surface.
- **Storefront**: dynamic-import the viewer, and only on products that
  actually have a mesh. Static import would tax the LCP of every product
  page in the estate for a feature a fraction of them use.
- **Cheaper alternative worth measuring first**: render a turntable —
  36 frames to a sprite sheet or a short MP4 — once, at generation time,
  and serve that. It is an `<img>`/`<video>`, it works on every device,
  it costs nothing per view, and for "does my figurine look right" it is
  most of the value. Rendering it server-side needs a headless renderer
  marketplace-api does not have, so for v1 generate the frames in the
  admin browser at publish time and store them.
- **Mobile storefront** is React Native: `model-viewer` is not an
  option, `expo-gl` + three is a real integration. The turntable works
  there unchanged, which is a second reason to prefer it.

## Endpoints

Admin:

- `POST /admin/stores/:storeId/products/:id/media/:mediaId/model3d` —
  case (c): generate from an existing product image. Returns the
  generation row, including a cache hit.
- `GET /admin/stores/:storeId/model3d/generations/:id` — status for
  polling the UI.
- `POST .../generations/:id/refine` — the paid second pass.
- `POST .../generations/:id/publish` — attach the mesh to the product.
- `GET .../generations/:id/asset` — signed GET for the GLB.
- `GET /admin/stores/:storeId/model3d/usage` — budget remaining, so the
  UI can warn before the merchant hits the wall rather than after.
- Order detail exposes case (b) generations per personalisation.

Storefront: **nothing, ever.** With case (a) dropped there is no
storefront endpoint in this design at any phase — the storefront reads
published meshes through the existing product media surface and knows
nothing about generations. That is most of what makes this design
safe.

`route-manifest.json` regeneration and a
`internal/handlers/storefront/route_parity_test.go` entry per admin
route (mobile-admin views and does not author) apply as ever.

## Testing

- `FakeGenerator` drives every service and worker test. No test reaches
  Meshy.
- Cache hit: same hash, params and version returns the existing row and
  **does not increment** `model3d_store_usage`. This is the test that
  protects the bill.
- Budget: the call is refused at the limit, before `Submit`.
- Orphan recovery: a job whose heartbeat went stale is re-polled, not
  re-submitted. Re-submission is the double-charge bug.
- Provider failure modes, each asserted: 4xx on submit, timeout, a
  completed job whose asset download 404s, malformed GLB.
- Asset custody: after completion no column anywhere holds a
  provider-hosted URL.
- Erasure and purge coverage for both new tables, plus the provider
  delete call.
- Error logging goes through `providerlog.SanitizeProviderError` —
  provider bodies are never logged raw.

## Risks

1. **Cost is unbounded without the budget gate.** Ship the gate in the
   same PR as the first `Submit` call, not after.
2. **Quality will disappoint for case (a)** more often than demos
   suggest. Single photos of people are the hardest input and the most
   likely one.
3. **Provider lock-in and churn.** Mitigated by the seam, not removed —
   `params_hash` and `model_version` are provider-shaped.
4. **Print-readiness for case (b).** The merchant receives a draft, and
   the UI must say draft. Single-image output is routinely non-manifold
   and unscaled; a merchant who treats it as a print file and ships the
   result has a refund, and will blame the platform that handed them the
   file. This replaces the dropped "a mesh is a promise to the buyer"
   risk, which case (a) carried and nothing now does.
5. **Provider retention of buyer photographs** may be incompatible with
   an erasure obligation. A phase-0 question with a real chance of
   changing the provider.
6. **Turnaround.** Minutes, sometimes longer under load. Any UI that
   implies "a moment" will be wrong; show a job, not a spinner.

## Delivery order

| # | Phase | Notes |
|---|---|---|
| 0 | Provider due diligence: DPA, retention, delete API, pricing, rate limits. Pick the provider on the answers | No code. Can change everything below. |
| 1 | `internal/model3d`: `Generator`, `FakeGenerator`, Meshy adapter, tables, `plangate` feature, usage counter | No callers. Fully testable. |
| 2 | Worker + dispatcher modelled on `internal/csvjob`, asset copy into the private bucket, orphan recovery | Still no UI. |
| 3 | Case (c): admin generate-from-product-image, status UI, publish | First shippable value, zero buyer exposure, bounded cost. |
| 4 | Admin viewer + turntable generation at publish | Measure the turntable against the viewer before shipping both. |
| 5 | Storefront turntable on published products, dynamically imported | Buyer-visible, still no buyer uploads involved. |
| 6 | Case (b): generation queued in `order.Service.Confirm`'s transaction, merchant download, overage prompt | Cost follows revenue. Last phase. |

Phases 3 through 5 deliver a real feature — "turn your product photos
into 3D" — with no buyer upload, no per-order cost and no promise to
anyone. If the provider turns out to be bad, that is where you find out,
for the price of a merchant's disappointment instead of a buyer's
refund. Phase 6 is then a small addition to a pipeline already proven in
production, rather than the first real use of an unproven one.
