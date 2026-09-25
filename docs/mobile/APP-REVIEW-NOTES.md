# App Store Connect — App Review Information

Paste the **Notes** section below into App Store Connect → App Review
Information → Notes, and fill the two bracketed gaps first. Apple asked for
items 1–7 after rejecting the previous submission under Guideline 2.1
(information needed, not a defect in the app).

Item 1 — the screen recording — cannot be drafted here. It must be captured
on a physical device running the current OS, starting from app launch, and
must show sign-in, the core flows, and every permission prompt.

Keep this file in sync when the app changes. Apple asks for the same notes
on every future submission.

---

## Notes field — copy from here

**2. Devices and operating systems tested**

> [FILL IN — e.g. iPhone 15 Pro (iOS 18.1), iPhone SE 3rd gen (iOS 17.6).
> List the physical devices the build was actually run on. Do not list
> simulators; Apple reviews on hardware and asks this to confirm you did
> too.]

**3. What the app does, and for whom**

Mark8ly Admin is the merchant-facing companion to Mark8ly, a hosted
e-commerce platform. Its users are shop owners and their staff — not
shoppers. There is no consumer-facing mode.

A merchant sells through a Mark8ly storefront on the web. This app is how
they run that shop away from a desk: checking orders as they arrive,
updating stock and prices, answering customer questions, and running
promotions. The problem it solves is that the work of a small shop does not
wait for the owner to be at a computer.

Core features:

- **Dashboard** — revenue this month, today and this week, customer count
- **Orders** — list and detail, fulfilment status, shipment tracking
- **Products** — catalogue, variants, pricing, stock, product images
- **Customers** — profiles, order history, and product reviews with replies
- **Marketing** — email campaigns, coupons, gift cards, a loyalty programme
  with referrals, and customer segments
- **Team** — invite staff and set their role
- **Support** — tickets raised by the merchant's own customers

**4. Setting up and accessing the main features**

Sign in with the demo credentials below. No setup, sample files or
configuration are required — the account is pre-populated with a live
storefront's data (around a dozen products, with real orders, customers and
reviews), so every screen has real content on first launch.

> Email: demo+appreview@mark8ly.com
> Password: as given in the Sign-In Information fields above.

The address is plus-addressed off the demo owner's inbox, so it delivers
somewhere we already read. It is a distinct account with its own
credentials, not an alias of the owner.

The account has the **admin** role on a demo store. That grants full access
to everything listed in item 3. It deliberately excludes deleting the store,
changing billing and managing domains, so the review account cannot destroy
the demo environment mid-review.

Sign-in options on the login screen: email and password, Sign in with Google,
and Sign in with Apple. Any of the three reaches the same app. The demo
account is configured for email and password.

This account is exempt from the new-device verification code that ordinary
merchant accounts receive, so no email inbox is needed to complete sign-in.

**Account registration, sign-in and deletion**

*Registration is not in the app.* Merchants create an account and their first
store on mark8ly.com, in a browser. The app is for merchants who already have
one. A signed-in user with no store is shown a "No store yet" screen with a
link out to the web; there is no in-app sign-up to demonstrate.

*Sign-in is in the app*, by email and password, Sign in with Google, or Sign
in with Apple.

*Account deletion is in the app*, under More → Account. It requires typing a
confirmation word and then confirming a second dialog that states the account
and — for a store owner — the store and all its products, orders and
customers are permanently removed.

The review account is an **admin**, not the store owner, so the final
deletion step is restricted for it by design: only an owner may delete a
tenant. The screen recording shows the flow as far as the confirmation
dialog and then cancels, because completing it would destroy the very demo
store these credentials are for.

**5. External services used to deliver core functionality**

- **Zitadel** — identity and authentication (self-hosted). Handles email and
  password sign-in, Sign in with Google, and Sign in with Apple.
- **Mark8ly platform API** — all merchant data: orders, products, customers,
  marketing. Operated by us.
- **Resend** — transactional email (order notifications, campaign delivery).
- **Google Cloud Storage, behind Cloudflare** — product image hosting.
- **Stripe** — subscription billing for the merchant's own Mark8ly plan.
  Not reachable from this app; billing is handled on the web admin only.

No AI or machine-learning services are used. No third-party data providers.
No advertising or analytics SDKs that track users across apps, and the app
does not request App Tracking Transparency permission.

**6. Regional differences**

The app functions identically in all regions. There is no
region-gated content, no regional feature flags, and no geographic
restriction on any screen.

Merchant storefronts may sell into different countries, and currency and tax
are presented according to the merchant's own configuration — but that is the
merchant's data being displayed, not a change in what the app does.

**7. Regulated industry and third-party material**

The app is business tooling for independent retailers. It is not a financial,
medical, gambling or otherwise regulated service.

It does not take payments. Card payments are processed on the merchant's
public storefront, on the web, by the merchant's own payment provider; this
app only displays the resulting order and payment status.

All content shown in the app is the merchant's own — their products, their
images, their customer records. The demo account's data is our own sample
storefront. No third-party protected material is included.

**Purpose strings and permission prompts**

The app asks for **one** permission: notifications. It is requested only when
the merchant turns push on under More → Settings → Notifications, never at
launch, and the app is fully usable if it is denied.

Adding a product image does **not** prompt. It opens the iOS system photo
picker (PHPicker), which runs outside the app and needs no photo-library
permission, so no dialog appears and the app never gains access to the
library. `NSPhotoLibraryUsageDescription` is declared because the binary links
the image-picker framework.

`NSCameraUsageDescription` is declared for product photo capture. That path is
not reachable from any screen in this build, so the camera is never opened and
no camera prompt appears.

**In-app purchases**

There are none. The app contains no StoreKit integration and sells nothing.
A merchant's subscription to Mark8ly is arranged on the web, outside the app,
and is not promoted or linked from within it. Guideline 3.1.2 therefore does
not apply.

---

## Before submitting — checks that caused the last rejection

| | |
|---|---|
| Demo credentials work on a device that has never signed in | the previous account required an emailed code the reviewer could not read |
| Demo account has a store with products | an account with no store is refused at sign-in with "We couldn't find a store for this account" |
| Screenshots show the app in use | not the login screen or title art (Guideline 2.3.3) |
| Build is current | test the exact build being submitted, on hardware |
