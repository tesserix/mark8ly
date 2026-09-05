# Stop the migration fast path claiming a domain-age check it does not run

#706. `internal/billing/migration/handler.go:112` gates the fast path on
`ValidateWhoisAge(ctx, domain, 90)`, rejecting with `whois_too_young`. That
rejection can never fire: production wires `NoOpValidator{}`
(`cmd/marketplace-api/main.go`), whose `ValidateWhoisAge` returns `nil`
unconditionally, and **no WHOIS or RDAP implementation exists anywhere** in the
repo.

So the system presents a substantive eligibility gate — "this merchant has
genuinely been operating elsewhere for 90+ days" — and enforces nothing.

## What is actually protecting this

The human CSM review at `POST /internal/csm/migration-fast-path/:id/review`.
That is a real control and this change does not touch it. The problem is that
the system reports the automated check as passed, so reviewer attention is
being spent on a check the reviewer may reasonably assume already happened.

## The decision

#706 offered implement-or-drop. Taking neither wholesale:

- **Not implementing RDAP here.** It is real work with an external dependency,
  and registrar privacy proxies mean the result needs a third state
  (verified / too-new / undeterminable) with undeterminable routed to a human
  — which is what already happens today for everything. Worth doing, but it
  should be its own change, decided on its own merits.
- **Not deleting the interface.** It is the correct seam for a real validator.

What ships now is honesty: the code should say plainly that domain age is NOT
verified, and it must be impossible to wire the no-op without noticing.

## Task

1. **Rename `NoOpValidator` to state what it does** — something like
   `UnenforcedDomainAge`. "NoOp" describes the implementation; the new name
   must describe the CONSEQUENCE, so `main.go`'s wiring line reads as an
   admission rather than a placeholder. Update the doc comment to say the
   90-day gate is not enforced and eligibility rests entirely on CSM review.
2. **Log loudly at startup** when it is wired — Warn or Error, naming the
   consequence, so an operator reading boot logs learns the gate is inert.
   This is #706's explicit ask: an unimplemented validator must be impossible
   to wire silently.
3. **Correct the misattributed comment.** `handler.go:20` says "TODO: wire
   P7's tax-ID package when it lands." The tax-ID package now EXISTS
   (`internal/billing/tax/`) and is irrelevant — it validates tax
   identifiers, not domain registration age. Anyone following that comment is
   sent to the wrong place. Say what is actually needed: a WHOIS/RDAP lookup.
4. Keep `PriorPlatformValidator` and the `handler.go:112` call site unchanged,
   so a real validator drops in without touching the handler.

## Out of scope

- Implementing RDAP/WHOIS.
- Any migration or new column. Recording "was the age verified?" on the review
  row so a CSM sees it is the substantive follow-up, and needs its own change.
- The CSM review flow itself.

## Done when

- Nothing in the tree claims a domain-age check is enforced
- Production logs the consequence at startup
- No behaviour change: the same requests are accepted, the same CSM review
  decides them
- `go build ./...`, `go vet`, `go test` green; existing migration tests pass
  unchanged

## Constraints

- Single atomic commit, single-line conventional message, NO attribution.
- SHARED CHECKOUT: no checkout/switch/stash/reset beyond this branch.
