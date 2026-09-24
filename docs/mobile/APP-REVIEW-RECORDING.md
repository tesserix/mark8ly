# App Review — screen recording script

Apple's rejection of 1.0 (submission `e02dd13d-2d6b-4ff5-a64c-512a943390ce`,
Guideline 2.1 – Information Needed) asks for seven things. Six are text and
live in [APP-REVIEW-NOTES.md](./APP-REVIEW-NOTES.md). This file is the
seventh — item 1, the screen recording:

> A screen recording captured on a physical device, running the latest
> operating system, demonstrating the app's functionality. The recording must
> begin with launching the app and show the typical user flow through its core
> features.

Apple then names four things to include if the app has them. This app has
three of the four, and the shot list below covers each one explicitly.

| Apple asks for | Where it is | Shot |
| --- | --- | --- |
| Account registration, login, deletion | Login screen; More → Account | 2, 9 |
| Paid content, purchase or subscription flows | **Not in this app** — see below | — |
| User-generated content, reporting and blocking | Reviews; customer blocking | 7, 8 |
| Prompts for sensitive data or device capabilities | Notifications — the only one the app raises | 6 |

There are no in-app purchases and no subscription flow in this app. Billing is
handled on the web, by the store owner, outside the app — `ITSAppUsesNonExemptEncryption`
is declared and there is no StoreKit integration to show. Say this in the
Notes field rather than leaving the reviewer to wonder why it is missing.

## Before you record

- **A physical iPhone on the latest iOS.** Apple asked for this specifically,
  and item 2 of the Notes field has to list the device and OS you actually
  used. Whatever you record on, write that down.
- **Use the iPhone's own screen recording**, from Control Centre. Do *not*
  record iPhone Mirroring on the Mac — it adds compression artefacts and drops
  frames, and the result looks like a broken app rather than a recorded one.
- **Install the TestFlight build**, not a dev build. A universal link only
  re-reads the AASA at install, so a fresh install of the new build is also
  the only way to see link handling work.
- **Sign out first**, so shot 2 starts from the real login screen.
- **Reset the notification permission**, or it will never prompt on camera:
  Settings → Mark8ly Admin → Notifications off, or delete and reinstall. That
  is the *only* permission prompt the app raises — see shot 5 for why photos
  does not, and do not go looking for a camera prompt that cannot happen.
- **Do Not Disturb on**, so a notification banner does not cover the app.
- Record **one continuous take** if you can. Apple is checking that the app
  runs, and cuts invite the suspicion that something was hidden between them.

Budget about four minutes. Pause two or three seconds on each screen once it
has loaded — a reviewer needs time to read it, and fast scrubbing reads as
evasive.

## Shot list

**1. Launch — from the home screen (0:00)**

Start the recording on the iOS home screen with the app closed. Tap the
Mark8ly Admin icon. Let the splash and first paint happen at real speed.
Apple asked that the recording *begin with launching the app*; starting inside
an already-open app is a common reason this gets bounced a second time.

**2. Sign in (0:10)**

The login screen offers email and password, Sign in with Google, and Sign in
with Apple. Say nothing about the last two — the Notes field already explains
they need the reviewer's own accounts.

Type the demo email and password by hand, slowly enough to be legible:

```
demo+appreview@mark8ly.com
```

Sign-in takes a few seconds — longer than you will want to sit through. Let it
finish rather than cutting; a jump from the login screen to a populated
dashboard reads as an edit.

This account is on `DEMO_LOGIN_EMAILS`, so it skips the new-device email code
that ordinary merchants get. There is no inbox to check and no MFA screen. If
a verification screen *does* appear, stop — that is a real regression and the
recording is not the problem.

**3. Dashboard (0:35)**

Lands on the store Dashboard. Let the figures load rather than cutting away
while they are skeletons. Revenue this month, today and this week, and the
customer count. Scroll to the bottom and back.

This is also the moment the reviewer sees the app has real content, which is
the point of item 4 of the Notes.

**4. Orders (0:50)**

Bottom tab → **Orders**. Show the list, then open one order. Scroll through
the detail: line items, totals, fulfilment status, shipment tracking. Go back
to the list.

Three orders exist on the demo store. That is enough to show the flow; do not
go hunting for a fourth.

**5. Products, and adding a photo (1:20)**

Bottom tab → **Products**. Show the catalogue — a dozen live products — then
open one and scroll the detail: variants, pricing, stock, images.

Then add media to a product and pick an image. The system photo picker opens
**straight away, with no permission dialog and no camera option**, and that is
correct rather than a bug: the app calls `launchImageLibraryAsync`, which on
iOS is PHPicker. PHPicker runs out of process and needs no library permission,
and `use-product-media-handlers.ts` deliberately does not ask for one — asking
opts into the legacy flow, where choosing "Limited Access" strands the user in
iOS's library-management sheet and the real picker never opens. There is a
regression test pinning that behaviour.

So: do not wait for a prompt here, and do not hunt for a camera button. The
app ships `NSCameraUsageDescription`, but the only component that calls
`launchCameraAsync` is `ProductMediaPicker`, which no route renders — the
camera is unreachable in the shipped app.

Let the picker open, choose an image, let the system cropper appear, and back
out without saving.

**6. Notifications permission (2:00)**

Bottom tab → **More** → **Settings** → **Notifications**. Toggle push
notifications on. **iOS prompts for notification permission here** — not at
launch, which is why it has to be reached deliberately. Allow it.

**7. Reviews — user-generated content and moderation (2:20)**

Bottom tab → **Customers** → **Reviews**. Open a review. Show that the
merchant can **approve** or **reject** it, and can reply.

This is the app's user-generated content and its moderation mechanism. Apple
asks for "content reporting and blocking mechanisms" — approve/reject on
reviews is the reporting half. Demonstrate it, but leave the review in the
state you found it.

**8. Blocking a customer (2:45)**

Bottom tab → **Customers**. Open a customer, or swipe the row, to reach the
**Block** action. Show the block-reason sheet, with its list of reasons.

**Cancel the sheet — do not block anyone.** Showing the mechanism is what was
asked for. Blocking a demo customer leaves the store in a worse state for the
next reviewer.

**9. Account deletion (3:05)**

Bottom tab → **More** → **Account** → **Delete account**.

Show the confirmation screen, type `DELETE` into the field, and show that the
button becomes enabled and a second confirmation dialog appears naming what
will be removed.

**Then cancel.** Do not complete it.

This needs to be said out loud in the Notes field, because a reviewer who sees
a cancelled deletion may assume the flow is broken. The reason is in
[APP-REVIEW-NOTES.md](./APP-REVIEW-NOTES.md): the review account is an
**admin**, not the store owner, and only an owner may delete a tenant —
completing it would destroy the very demo store the credentials are for. The
flow is demonstrated as far as it can honestly go.

**10. Close (3:30)**

Return to the Dashboard so the recording ends somewhere coherent, then stop
the recording.

## After recording

1. Watch it back the whole way through before uploading. Check the credentials
   are legible, both permission prompts are on screen, and nothing personal
   appears in a notification banner.
2. Note the **exact device model and iOS version** and put them in item 2 of
   the Notes field — that is one of the two `[FILL IN]` gaps.
3. Attach the recording in App Store Connect under App Review Information →
   Attachment, and reply to the Aug 17 message in the Resolution Center rather
   than only resubmitting silently.
