---
id: 260908-pa1
slug: proapp-teardown
date: 2026-09-08
issue: 702
kind: quick
branch: feat/702-discover-apple-app-at-teardown
---

# A cancelled Pro+App merchant's App Store listing actually gets pulled (#702)

When a Pro+App merchant cancels, `finalize.go:88` emits
`subscription.pro_app_cancelled` and then logs that it "would notify" a service
that does not exist. The consumer that would retire the app is written, tested,
and **never constructed**. Nothing seeds `white_label_app_state`, so the advancer
runs forever over an empty table and the merchant's app stays live indefinitely.

## Three gaps, not the two the issue names

Measured 2026-09-08:

1. **The consumer is never constructed** and there is no delivery path from the
   emit to `Handle`. (#702 names this.)
2. **No source for the identifiers it needs.** (#702 names this.)
3. **Two of the three teardown clients are unimplemented stubs.** `googleplay`
   and `firebase` return `ErrNotWired` from every method. Only `apple` is real —
   it makes genuine ASC calls with ES256-JWT auth. **#702 does not name this**,
   and it caps what any amount of wiring can achieve.

## Scope, decided 2026-09-08

**Apple only, and the limitation is stated rather than implied.**

This makes App Store teardown genuinely work — the surface with the strongest
obligation, since a published listing under merchant branding is the most
visible thing we keep operating for someone who has stopped paying. Play and
Firebase remain `ErrNotWired`.

**#702 does not close on this.** It goes from "nothing is torn down" to "the App
Store listing is retired; Play and Firebase are not, visibly."

## Decisions, settled — do not re-open these

1. **Discover, do not record.** Identifiers are resolved at teardown from the
   credentials already stored, not written down at provisioning — there is no
   provisioning flow to hang that on. See #702's analysis comment.
2. **Discovery must run BEFORE the day-90 credential purge.** `app_credentials.go`
   states the advancer purges all four credential types at day 90. A teardown
   that reaches `credentials_purged` without having discovered anything has
   permanently lost the ability to. Seed at cancel time, not lazily.
3. **Ambiguity fails loudly.** If ASC returns more than one app, do NOT guess.
   A wrong app id pulls a listing belonging to a merchant who did not cancel,
   which is worse than not pulling one. Refuse with a named error, the same
   discipline `ErrNoAppIdentifiers` (#711) already set.
4. **`GooglePackage` stays empty.** There is no source and the client is a stub;
   inventing one would seed a row that walks the state machine reporting
   teardown of something that never happened.
5. **`FirebaseProjectID` comes from the service-account JSON's `project_id`**,
   which `ValidateGooglePlayJSON` already parses and discards. Record that it is
   the *GCP* project of the service account — usually the Firebase project for a
   Firebase-backed app, not guaranteed. The Firebase client is a stub anyway, so
   nothing acts on it yet; capturing it now costs nothing and is honest about
   what it is.
6. **The delivery path is an interface the emitter owns**, not pub/sub and not a
   direct import of `whitelabel/lifecycle` from `subscription/lifecycle`.
   `FinalizeCron` gains a small notifier interface it defines; `main.go` wires
   the real consumer into it. Pub/Sub is what `finalize.go`'s comment defers to,
   and it is not needed for an in-process call.

## THE LESSON THIS TASK EXISTS UNDER

The failure being fixed is *a system reporting teardown of something it never
touched*. Every part of this must refuse to do a smaller version of that:

- a row seeded with no Apple id must not advance as though it tore something down
- Play and Firebase returning `ErrNotWired` must be **visible on the row and in
  the logs**, never swallowed so the state machine can proceed to
  `credentials_purged`
- ambiguous discovery must refuse, not pick

## Tasks

- **T1 — `ListApps` on the Apple client.** `GET /v1/apps` using the existing
  `call`/auth path, on `ClientAPI`, the real `Client`, and `FakeClient`. Returns
  the app ids the merchant's ASC credentials can see.
- **T2 — discovery + the delivery path.** A notifier interface on `FinalizeCron`;
  `main.go` constructs `ProAppCancelledConsumer` and wires it. On cancel:
  discover the Apple id (refusing on 0 or >1), read the SA `project_id`, seed the
  row. Assert the emit still happens when discovery fails — a failed teardown
  must not swallow the audit event.

## Done means

- [ ] A cancelled Pro+App store seeds `white_label_app_state` with a real Apple id
- [ ] More than one ASC app refuses by name; zero refuses by name
- [ ] The audit event is emitted whether or not discovery succeeds
- [ ] Play and Firebase `ErrNotWired` is visible, and does not advance the row
- [ ] #702 updated: what now works, what does not, and that it stays open
