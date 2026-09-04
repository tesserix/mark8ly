# mark8ly-owned MCP connectors

Design for milestone 9 (MCP: platform agent integration). Written 2026-09-04.

Every claim marked **VERIFIED** was observed against the live cluster, the
running registry seeds, or the repos in this workspace on 2026-09-04 — not read
from documentation.

## The problem is ownership, not wiring

mark8ly appears to have an MCP integration. It does not have one it owns.

**VERIFIED.** `mark8ly-mcp` runs in the `mark8ly` namespace (113d old, Healthy),
but its image is
`asia-south1-docker.pkg.dev/.../tesserix/slm-support-platform/mcp-gateway`, and
the chart that deploys it (`charts/apps/mcp-gateway` in `tesserix-k8s`) says so
in its own values file: *"one shared image for every tenant."* The eight tools
it serves — `get_order`, `list_returns`, `list_recent_orders`,
`check_payment_status`, `create_refund_request`, `create_support_ticket`,
`search_knowledge_base`, `lookup_conversation` — are implemented in
`slm-support-platform`, a repository not present in this workspace. mark8ly is
differentiated from homechef, platform and stockpilot by a `tenant` Helm
parameter.

mark8ly's entire own contribution to its agent surface is one 187-line
hand-written OpenAPI document,
`services/marketplace-api/internal/handlers/storefront/openapi.go`, ingested at
gateway startup. **VERIFIED: it has never produced a single tool.** #412
recorded the cause (a URL missing both `:8080` and the `/api/v1` prefix); that
URL is now corrected on `origin/main` and the spec returns **200** from inside
the cluster, but the registry record for `mark8ly-mcp` lists only the eight
gateway-native tools and none of the spec's five operations. The catalog surface
the spec was written to enable has never existed.

This is the shape australis names as its two principal failure modes: the
**god-server** ("a god-server accumulates unrelated scopes, and scopes are the
unit of authorisation") and the shared failure domain that its invariant 1
exists to prevent. One image change ships four products at once.

## What already exists, and is worth keeping

The estate has more MCP infrastructure than mark8ly uses, and it is already
shaped the way australis specifies.

| Thing | State | Where |
| --- | --- | --- |
| AgentGateway | **running** — incl. a dedicated `agentgateway-mcp` | ns `agentgateway-system` |
| MCP CRDs | `mcpservers.kagent.dev`, `remotemcpservers.kagent.dev` installed | cluster-wide |
| Registry | `agentic-registry` — a catalog, explicitly never a proxy | its own repo |
| Registry seeds | 31 catalog + 12 product records, incl. `mark8ly-mcp.yaml` | `devai/architecture/registry-seeds/mcp-servers/` |
| Hub implementation | registry-driven discovery, SSRF guard, identity terminated at hub | `devai/src/devai/mcphub/` |
| Label vocabulary | `mcp.tesserix.app/tenant`, `.../class` — already on mark8ly's ArgoCD app | estate-wide |

**VERIFIED: zero `MCPServer` custom resources exist estate-wide**, and devai's
catalog seeds declare `registry.solo.io/v1alpha1` while the installed CRD group
is `kagent.dev`. The seeds are a source of record that has never been applied to
the cluster. That is out of scope here (see Non-goals) but it means a seed is
today a *document*, not a deployed object — so this design treats the registry
record as the reviewable contract it is, and does not depend on the CR existing.

The per-product runtime split is already correct: `homechef-mcp.homechef.svc`,
`mark8ly-mcp.mark8ly.svc`, each with its own key
(`product-mcp-upstream-keys` / `MARK8LY_MCP_KEY`, header `X-MCP-Key`). Ownership
of the *process* is already mark8ly's. Only the *source* is misplaced.

## Decisions

| # | Decision |
|---|---|
| D1 | mark8ly's MCP servers live in `mark8ly`, not in australis. ADR-0001 D1 is amended, not ignored. |
| D2 | The reusable foundation is `go-shared/mcp`, from day one. Each server is a thin domain package over it. |
| D3 | `catalog` is the first server. Support and platform-read follow, in that order. |
| D4 | Tools read through marketplace-api's storefront HTTP surface, never the database directly. |
| D5 | Every tool declares closed input **and** output schemas. No pass-through of upstream JSON. |
| D6 | Read-only in v1, enforced structurally: the foundation cannot express a non-GET upstream call. |
| D7 | One server per bounded domain, one Deployment, one ServiceAccount, one key. |
| D8 | Registration is a reviewed registry record with a pinned image digest. No `latest`, ever. |
| D9 | `go-shared/mcp` carries no MCP SDK dependency. The protocol binding stays in the consuming service. |

### D1 — the servers live here, and the ADR is amended

australis ADR-0001 D1 says connectors are built from australis
(`servers/mark8ly/catalog`, `servers/mark8ly/orders` are named in its layout).
**VERIFIED: australis has no code — `servers/` contains one README, and the repo
describes itself as "design / pre-implementation".** Making mark8ly's first
connector the first thing ever built there means inheriting a repo with no CI,
no images and no runtime, to solve a problem that is about which repo the *tools*
live in, not which repo the *design* lives in.

australis's own D5 argues the same way: *"Co-locating the code must not transfer
ownership of the domain knowledge in it. The person who knows that
`daily_log_summary` must exclude soft-deleted entries is the person who reviews
changes to it."* The person who knows a mark8ly refund must file a return in
`requested` state and never move money is here.

What matters about australis is its **contract**, not its address — closed
schemas, read-only v1, `credentialRef` only, digest pins, AgentGateway
invocation, one server per bounded domain. Every one of those is satisfiable
here, and this design satisfies all of them.

**This divergence must be recorded as an amendment on ADR-0001**, or the next
reader will find D1, see mark8ly, and "fix" it back. That amendment is a
deliverable of this work, not an afterthought.

### D2 — the foundation is `go-shared/mcp`, from day one

The point of this design is not one catalog server. It is that the second and
third servers — and tesserix-home's — cost days rather than weeks.

**VERIFIED: tesserix-home has the identical problem.** Its `platform-mcp` record
points at `platform-mcp.support-platform.svc.cluster.local`, i.e. it runs in the
`support-platform` namespace on the same shared `slm-support-platform` image. It
does not own its tools either, and it has no MCP code of its own. A second
consumer is not hypothetical; it already exists and is already broken.

So the foundation goes straight into `go-shared` rather than being built in
mark8ly and extracted later. **VERIFIED: go-shared is at v1.9.1 and already
holds 25 packages of exactly this character** — `httpclient`, `serviceclient`,
`middleware`, `metrics`, `signature`, `authz`. An `mcp` package is not a foreign
body there.

```
go-shared/mcp/                   the foundation — domain-free AND SDK-free
├─ schema/                       Go type -> JSON Schema; a tool cannot register
│                                without BOTH an input and an output type
├─ upstream/                     GET-only HTTP client, per-call deadline (D6)
├─ auth/                         X-MCP-Key verification, fail-closed
└─ observe/                      per-tool latency, outcome, upstream-failure metrics

mark8ly services/mcp/
├─ internal/server/              binds go-shared/mcp to the MCP SDK + transport
├─ internal/catalog/             server #1 — 5 tools over the storefront reads
├─ cmd/mcp-catalog/              one binary, one image, one Deployment
└─ (later) internal/support/, internal/platformread/ + their own cmd/ + images
```

`go-shared/mcp` may not import any product package, and this is asserted by a
test in go-shared rather than left to convention — the same shape as australis's
invariant 2.

**The cost of choosing day one over extract-later, stated plainly.** The
foundation's API will churn most in its first weeks, and every churn is now a
go-shared release plus a bump in mark8ly — a two-repo loop before the first
server works, inside a library every Go service in the estate imports. That was
weighed and accepted: the counter-argument is that an extraction deferred until
a second consumer appears is an extraction that frequently never happens, and
here the second consumer demonstrably already exists.

D9 exists to keep that cost bounded.

### D3 — catalog first, because nothing there can regress

Three candidate surfaces; the ordering is a risk argument.

**Catalog is first because it is provably dead.** Its five operations have never
registered as tools, so there is no behaviour to preserve, no consumer to break,
and any outcome is an improvement. It is the cheapest possible first exercise of
an unproven pattern, and australis's own layout names `catalog` as mark8ly's
first connector.

**Support is second.** Its eight tools are live, in production, and answering
real customer questions. Migrating them off the shared image is a
behaviour-identical rewrite — the right second task, and the wrong first one.

**platform-read is last**, and deliberately so. It is cross-tenant by
construction (#408, #411), and its auth question (#409 — the admin surface is
HMAC-signed per request with a single-use nonce, which a static header cannot
express) is the hardest decision in the milestone. Two of #409's three options
become unnecessary if invocation goes through AgentGateway, so deciding it before
the pattern exists risks deciding it twice.

### D4 — read through marketplace-api, never the database

The catalog tools call marketplace-api's storefront engine over HTTP with
`X-Storefront-Key`, the same credential the gateway is already configured with.

The alternative — querying `marketplace_db` directly — is rejected. Published
filtering, tenant scoping and store-slug resolution are business rules that live
in marketplace-api's handlers. A second reader of those tables would duplicate
them, and would drift the first time one changes. One data owner.

The cost is an in-cluster HTTP hop, which is comfortably inside the 400 ms budget
australis sets. If a future tool cannot meet that budget, australis's own answer
applies: pre-materialise the aggregate on marketplace-api's side and have the
tool read the materialised row. The model never does arithmetic.

### D5 — closed output schemas are the whole point

australis invariant 4: *"Every tool declares closed input and output schemas — an
untyped result cannot be cited."*

This is precisely what the OpenAPI-ingestion approach cannot deliver, and the
reason fixing #412 in place was rejected. The spec describes operations'
*inputs*; the gateway derives a tool whose output is whatever the endpoint
returned. A grounded answer with a citation needs a typed, stable result.

So each tool defines a Go result struct, and the tool returns *that* — never the
upstream body. Fields marketplace-api returns that an agent has no business
seeing (internal ids, cost prices, inventory internals) are dropped by
construction, because they are absent from the struct rather than removed by a
filter someone must remember to update.

This is also the projection boundary #411 asks for, arriving early: an operation
is exposed because someone opted it in, and a field is exposed because someone
typed it.

### D6 — read-only, structurally

The foundation's upstream client exposes `Get(ctx, path, params, &out)` and
nothing else. There is no method that issues a POST, PUT, PATCH or DELETE.

This is deliberately stronger than a review rule. #408 asks that the generator be
"incapable of emitting a non-GET operation ... rather than relying on an author
to leave them out". The same reasoning applies to the server: a write is not
something an author must remember to avoid, it is something the code cannot
express. A test asserts the client's method set.

When writes eventually arrive (`create_refund_request` is a write, and it is in
the support server's scope), they arrive as a deliberate, separately reviewed
capability on that server — not as a quiet widening of this one.

### D7 — one server, one deployment, one identity

Per australis invariants 7 and 8: `mcp-catalog` gets its own image, its own
`Deployment` and KEDA `ScaledObject` (min 1, max 5 — no scale-to-zero, because a
cold start does not fit a 400 ms budget), its own ServiceAccount, and its own
`X-MCP-Key`. Transport identity and data identity name the same unit.

**Cost, stated honestly.** Until the support tools migrate (D3, step 2), mark8ly
runs two MCP pods: the existing shared `mark8ly-mcp` and the new `mcp-catalog`.
Against this project's "minimize GCP costs" constraint that is a real, temporary
+1 pod. It is temporary rather than additive: when support migrates, the shared
pod is deleted and mark8ly runs only servers it owns.

### D8 — no `latest`, on either axis

australis ADR-0001 D3 forbids resolving `latest` at request time and requires
pinning both `registry_digest` and `artifact_digest`.

mark8ly's deploy chain already agrees on the image axis: Kargo writes a concrete
`main-<sha7>` tag into the ArgoCD Application's Helm parameter. This design adds
the other half — the registry record for `mark8ly-mcp-catalog` names the pinned
image digest and the tool list, and is reviewed when either changes. A tool
appearing in the server but not the record is a review failure, and a test in CI
compares the two.

That test is the direct answer to #412's real ask ("assert the tools actually
register"), generalised: the failure there was not a typo, it was that **nothing
compared what was declared against what was served.**

### D9 — the SDK dependency stays out of go-shared

`go-shared/mcp` contains no MCP SDK import, and no protocol types. It carries the
schema registry, the GET-only upstream client, key verification and metrics —
all of which are plain Go over the standard library and go-shared's existing
dependencies. The binding to a specific MCP SDK and protocol version lives in
the consuming service, at `services/mcp/internal/server`.

Two reasons, and the second is the load-bearing one.

**Supply chain.** Every Go service in the estate imports go-shared. An SDK
dependency there enters ~30 services' module graphs whether they serve MCP or
not, and each becomes a surface for the CVE bumps this repo already deals with
routinely.

**Churn lands in the wrong place.** The protocol is where the movement is — the
estate's registry records pin `protocolVersion: '2026-07-28'`, and a protocol
revision would otherwise force a go-shared release affecting every service, to
change something only MCP servers care about. Keeping the binding in the service
means a protocol bump is a mark8ly change; only genuine foundation improvements
become go-shared releases. That is what keeps D2's accepted two-repo cost from
compounding.

A test in go-shared asserts the `mcp` package's import graph contains no MCP SDK.

## Architecture

```
agent ──► AgentGateway ──► mcp-catalog :8765/mcp        (X-MCP-Key, fail-closed)
                                │
                                │  tool call: input struct ─► validated
                                ▼
                     internal/catalog  ──► go-shared/mcp upstream (GET-only)
                                                    │  X-Storefront-Key, 400ms deadline
                                                    ▼
                       marketplace-api-storefront :8080/api/v1/storefront/...
                                                    │
                                                    ▼
                              projected into a closed result struct ─► agent
```

### The five tools

Derived from the existing spec's operations, renamed to MCP tool conventions and
given result types. Store scoping is by **slug**, never by `store_id` — the slug
is the public identifier, and accepting an internal id would invite
cross-store probing.

| Tool | Upstream | Result carries |
| --- | --- | --- |
| `list_store_products` | `GET /storefront/stores/{slug}/products` | handle, title, price + currency, short description, availability |
| `get_store_product` | `GET /storefront/stores/{slug}/products/{handle}` | the above + images, variants, in-stock state |
| `list_store_categories` | `GET /storefront/stores/{slug}/categories` | slug, name, product count |
| `list_products_by_category` | `GET /storefront/stores/{slug}/categories/{slug}/products` | as `list_store_products` |
| `get_store_branding` | `GET /storefront/stores/{slug}/branding` | display name, logo URL, primary colour, active promotions |

`limit` is clamped server-side to the spec's stated max of 100 regardless of what
the agent asks for, and a rejected value is an error, not a silent clamp — an
agent that believes it received 500 products and received 100 will summarise a
catalogue it never saw.

### Error handling

Three outcomes, kept distinct for the same reason the parity monitor keeps
"compared" separate from "differences":

- **Not found** (unknown store slug, handle or category) — a typed empty result
  with an explicit `found: false`, never an empty list. An empty list reads as
  "this store sells nothing", which is a plausible-looking wrong answer.
- **Upstream unavailable** — an MCP error. The agent degrades to a
  document-only answer with disclosure, per australis. It must never be
  reported as an empty or partial catalogue.
- **Deadline exceeded** — surfaced as unavailable, and counted separately in
  metrics so a slow upstream is distinguishable from a down one.

No tool returns a partially-populated result. A projection that could not be
completed is a failure, not a degraded success.

## Testing

- **Schema conformance** — every registered tool has both an input and an output
  schema, asserted by walking the registry. A tool registered without one fails
  the suite; it cannot reach production.
- **Read-only, structurally** — a test asserts the upstream client's exported
  method set contains no write verb.
- **Foundation isolation** — two tests, both in go-shared: `mcp` imports no
  product package, and its import graph contains no MCP SDK (D9).
- **Declared-vs-served** — CI compares the server's tool list against the
  registry record. This is #412's lesson, made permanent.
- **Upstream contract** — the projection is tested against recorded real
  marketplace-api storefront responses, so a field rename upstream fails here
  rather than silently emptying a result. This replaces the existing spec's
  drift tests, which go away with the spec.
- **`go build ./... && go vet ./... && go test -race ./...`** must pass in
  `services/mcp`, and the service is added to `.github/workflows/ci.yml`'s Go
  matrix (`service: mcp`, `directory: services/mcp`). The existing floors range
  from 4 to 31, which is not a precedent to copy — a greenfield service with a
  five-tool surface starts at **60** and the number goes up, never down.

## Sequencing

1. `go-shared/mcp` — schema registry, GET-only upstream client, key auth,
   metrics — released as a go-shared minor version. No mark8ly change yet.
2. `mcp-catalog` in mark8ly against it, deployed alongside the existing shared
   gateway. Prove the pattern end to end: a real agent lists a real store's
   products.
3. Amend australis ADR-0001 with D1's reasoning.
4. Migrate the eight support tools to `internal/support` + `cmd/mcp-support`,
   behaviour-identical. Delete the shared `mark8ly-mcp` deployment. Cost returns
   to where it started.
5. Only then take up platform-read (#408, #409, #411), with AgentGateway
   invocation as the starting assumption for the auth question.

Steps 1–4 are independently shippable and each leaves the system working.
tesserix-home's `platform-mcp` is a separate consumer of step 1's output and is
not sequenced here — it is that repo's decision, informed by a filed issue.

## Non-goals

- Applying the registry seeds as cluster CRs, or reconciling devai's
  `registry.solo.io` seeds with the installed `kagent.dev` CRD group. Real, and
  a separate piece of work.
- Changing the shared `charts/apps/mcp-gateway` chart. mark8ly stops using it
  rather than modifying something three other products depend on.
- Anything on the storefront's own request path. No customer-facing behaviour
  changes.
- Writes, MFA, or agent-initiated actions of any kind.
- Migrating homechef, platform or stockpilot. If this pattern is good they will
  want it; that is their decision, not this design's.

## Open questions

1. **Does AgentGateway front these servers today, or do callers dial the Service
   directly?** The registry records name the in-cluster Service URL, and D4 of
   ADR-0001 forbids direct pod-to-pod. Worth establishing before step 4, because
   it is most of #409's answer.
2. **Which Go MCP SDK.** mark8ly has none today (VERIFIED: no MCP dependency in
   any `go.mod`). The official `modelcontextprotocol/go-sdk` is the default
   assumption, and it must support `2026-07-28` — the protocol version every
   product record in the registry pins. Under D9 this is a mark8ly decision
   rather than a go-shared one, so it blocks step 2, not step 1.
3. **Is `search_knowledge_base` mark8ly's at all?** It appears in both the
   mark8ly and homechef records with the same name. If it is genuinely shared
   infrastructure it should not migrate in step 3 — it is not a mark8ly bounded
   domain.
