# Customer store membership — design

**Status:** proposed
**Context:** #524 (Zitadel migration) surfaced this; the behaviour predates it and is not caused by it.

## Problem

A customer who signs up at one store can sign in at any other store on the platform and silently acquire an account there.

Verified on 2026-09-05:

- **One identity pool.** GIP used a single customer tenant (`MP-Customer-39opy`); Zitadel uses a single org. One email is one account, platform-wide.
- **No store scoping in the credential check.** `auth-bff`'s customer handler has no notion of a store — it "verifies the credential against Zitadel and returns `{uid, email}`. Full stop."
- **Membership is created as a side effect.** `EnsureProfile` runs from `storefront/middleware.go` on *any* authenticated request and inserts a `customer_profiles` row if none exists. Nothing requires one to pre-exist.

So visiting store2 with a valid password is enough to become a customer of store2.

This is not observable today — of three stores, only `the-bondi-store` has customers (5 profiles, 5 distinct emails, 1 distinct store) — which makes now the cheap moment to fix it.

### What is already isolated

Worth stating, because the gap is narrower than "no isolation":

| Layer | Isolated per store |
|---|---|
| Session cookie (`mp_customer_session`) | **yes** — HMAC-scoped to the exact request host |
| Profile, orders, addresses, loyalty | **yes** — `UNIQUE (store_id, email)`, all keyed on `store_id` |
| Identity and password | **no** — one account per email, platform-wide |

The per-store cookie scoping is deliberate: auth-bff's own doc says "a customer signed in on one store's subdomain must never be handed a session usable on another store". That property holds. What does not hold is preventing a *fresh* session being minted at store2.

## Decision

**One platform identity, explicit per-store membership.**

A customer has a single Mark8ly login. Access to each store is a distinct membership they must deliberately join. Signing in at a store they have not joined does not grant access.

This is the model Shopify moved to, and it fits a platform of branded storefronts where the merchant reasonably considers the customer relationship theirs.

### Rejected: an identity per store

True identity isolation would mean **one Zitadel org per store**, because Zitadel enforces email uniqueness within an org — confirmed live: creating a second user with an existing address fails with `User already exists (V3-DKcYh)`.

That buys genuinely separate credentials but costs org provisioning per merchant, per-org policy templating, and host→org resolution at login. It is disproportionate to the problem, and it makes one customer maintain N passwords for N stores.

### What this decision does NOT give you

State plainly, because the UX must not claim otherwise:

- **The password is shared.** A reset at store1 changes the credential at store2. This is correct under a platform-identity model, but it is wrong under a "separate accounts" story. The UI must say *"your Mark8ly login works here"*, never imply a distinct account.
- **Existence is disclosed across merchants.** Register at store2 with a known address reveals that the address has a platform login. Under this model that is intended behaviour ("you already have a login — sign in to join"), not a leak; under a separate-accounts story it would be one.

If either is unacceptable, the decision must be revisited *before* implementation, because the fix is org-per-store, not a variation of this design.

## Design

### The membership record

`customer_profiles` already is the membership record: `UNIQUE (store_id, email)`, per-store, with a `status` column. No new table.

The change is that a row must be created **deliberately**, not incidentally.

### Sign-in

Sign-in at a store the customer has not joined must not mint a session. It should offer the join instead, and the copy must not imply a wrong password.

### Joining

An explicit action — the customer is signed in (or signs in) and confirms they want an account at this store. It creates the `customer_profiles` row and nothing else: no new identity, no new credential.

### Registration at a second store

Register with an address that already has a platform login should route to join rather than fail with `email_taken`. The current copy is actively misleading here.

### `EnsureProfile` must stop creating

This is the crux. It runs from storefront middleware on any authenticated request, so leaving it as-is defeats every gate above. It should become read-only for the session path, with creation moved behind the explicit join.

**Check the mobile path too** — `storefront/mobile_stubs.go` and `mobile_routes.go` call `EnsureProfile` independently, and mark8ly has shipped mobile apps under `apps/`. A gate applied only to the web storefront would leave the mobile bearer path creating memberships silently.

## Edge cases to settle during planning

- **Guest checkout.** If orders can exist without a profile, does completing a guest order create a membership? It probably should — the customer transacted with the store.
- **Existing customers.** The 5 live profiles are all on one store; no migration needed. Verify before implementing rather than assuming it is still true.
- **Blocked customers.** `status`/`block_reason` already exist. A blocked customer must not be able to re-join and reset their state.
- **Admin-created customers.** If a merchant can add a customer from the admin, that is a membership created without the customer's action — decide whether it counts as joined.

## Non-goals

- Org-per-store identity isolation (see Rejected)
- Putting storefront customers into OpenFGA. They are deliberately excluded: adding them would need either a bypass flag in the merchant authorization path or tuples they have no reason to hold. Membership stays a `customer_profiles` row.
- Any change to the merchant/admin side.

## Acceptance

1. A customer with an account at store1, signing in at store2, does **not** get a session and is offered the join.
2. After joining, they sign in normally and their store1 data is not visible at store2.
3. The mobile bearer path enforces the same gate.
4. No membership is created by browsing, by an authenticated API call, or by any path other than the explicit join (or a deliberate decision made for guest checkout).
5. Register at a second store routes to join instead of `email_taken`.

Point 4 is the one to test adversarially: the current bug exists precisely because membership creation was a side effect nobody had to ask for.
