# Wire the Android Publisher client — for the one step that is possible (#702)

## What was asked, and what the API actually allows

`internal/whitelabel/googleplay` is a frozen-interface stub: `BlockDownloads` and
`PullApp` both return `ErrNotWired`. The task was to wire it. Verified against
Google's reference on 2026-09-09, only one of the two can be:

| step | doc comment claims | reality |
|---|---|---|
| Day 30 `BlockDownloads` | "update the default track's rollout so no new downloads" | **Implementable.** A production-track release set to `halted` — "The release's APKs will no longer be served to users" — is exactly this, and existing installs keep working. |
| Day 60 `PullApp` | "remove the app listing via `edits.tracks.update` flipping state to **unpublished**" | **Not implementable.** There is no `unpublished` status; the four values are `draft`, `inProgress`, `halted`, `completed`. `edits.tracks.update` manages releases within a track and cannot delist. `applications` has exactly one method, `dataSafety`. Unpublishing is a Play Console action with no API. |

So the package's own doc comment describes a mechanism that does not exist. That
is the defect class this estate keeps finding: a comment stating a reason, or here
a mechanism, that is simply false — and it is the reason nobody noticed day 60 was
unbuildable.

**The dependency note is also stale.** The header says adding
`google.golang.org/api/androidpublisher` "is a follow-up"; `go.mod:44` already has
`google.golang.org/api v0.287.1` and `go.mod:40` has `golang.org/x/oauth2 v0.36.0`.
Nothing new to add.

## Scope

Wire `BlockDownloads` for real. Make `PullApp` honest. Correct the comments. Do
**not** invent a day-60 mechanism, and do not quietly redefine `PullApp` to mean
`BlockDownloads` — a cancelled merchant's listing staying up is a fact the
platform should state, not paper over.

## Tasks

### 1. Wire BlockDownloads against Android Publisher
- [ ] Service-account OAuth2: sign a RS256 JWT from the stored service-account
      JSON, exchange for a token, scope `androidpublisher`. `internal/whitelabel/
      apple/client.go` is the template for shape — credentials fetched at call
      time and never held on the Client (spec §18.9), a token TTL, and a typed
      error for auth failure (`apple/client.go:63` distinguishes revoked keys).
- [ ] The Publisher edit lifecycle: `edits.insert` -> `edits.tracks.get` /
      `edits.tracks.update` setting the production release `status: "halted"` ->
      `edits.commit`. An abandoned edit must not leak: `edits.delete` on any
      failure path after insert.
- [ ] Idempotent, as the interface promises: a package already halted must
      succeed, not error.
- **Done when** a halted production track is observable, re-running is a no-op,
  and a revoked service account surfaces a distinguishable error rather than a
  generic one.

### 2. Make PullApp fail honestly
- [ ] Replace `ErrNotWired` with a distinct, documented error naming the real
      constraint: unpublishing has no Android Publisher API and requires a Play
      Console action. Do not return success.
- [ ] The advancer already tolerates a Play-side failure as "skip for this run"
      (`googleplay/client.go:15`, `advancer.go:256-258`). Check what that means
      for a row that can now NEVER complete day 60, and make sure the outcome is
      recorded rather than retried silently forever — the same reasoning
      `advancer.go` already applies to the Firebase stub, which records that an
      archive did not happen rather than letting the status assert one.
- **Done when** nothing in the system claims a Play listing was pulled, and the
  operator-visible record says why it was not.

### 3. Correct the false comments
- [ ] The package header's `PullApp` mechanism claim, and the stale dependency
      note.
- [ ] State what IS possible and what is not, so the next reader does not
      re-derive it from Google's reference.

## Out of scope, and why

- **The package name.** Nothing produces one; `GooglePackage` is deliberately
  empty (`discovery.go:99`, "decision 4"). So both methods are unreachable in
  production until that is decided — the Publisher API cannot list apps, and the
  only discovery path is Play Developer Reporting `v1beta1 apps:search` on a
  different scope. Wiring the client is worth doing anyway: it is needed under
  every option, and it converts "unwired" into "waiting on one input".
- **Firebase.** `firebase/client.go:35` is a separate unwired stub.
