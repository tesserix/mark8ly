# Sign in with Apple — web admin

The web sign-in code is complete and merged. The button is **hidden until
configured**, because Apple treats web as a separate client from the iOS app
and a button without a Services ID fails at Apple rather than in our code.

Native (mobile-admin) already works — it uses the bundle ID
`com.mark8ly.admin`. Web needs its own client. That is the only gap.

## Current state

```
apple.com    enabled:      true
             bundleIds:    ["com.mark8ly.admin"]   # native, working
             clientId:     EMPTY                   # web Services ID — missing
             clientSecret: EMPTY                   # derived from a .p8 key — missing
```

## Setup (Apple Developer account required)

Nobody outside the Apple Developer account can do these steps.

### 1. Create a Services ID

Certificates, IDs & Profiles → Identifiers → **+** → **Services IDs**

- Description: `Mark8ly Admin Web`
- Identifier: `com.mark8ly.admin.web` (this becomes the web `clientId`)

### 2. Configure it for Sign in with Apple

Enable **Sign in with Apple** on the Services ID → **Configure**:

- Primary App ID: the existing `com.mark8ly.admin`
- Domains and Subdomains: `tesseracthub-480811.firebaseapp.com`
- Return URLs: `https://tesseracthub-480811.firebaseapp.com/__/auth/handler`

> The return URL points at **GIP's** handler, not at our own domain. That is
> deliberate and load-bearing: Apple does **not** support wildcard domains,
> so if the return URL lived on `{slug}-admin.mark8ly.com` every tenant
> subdomain would need registering by hand. Routing through GIP's fixed
> handler means one domain covers every tenant.

### 3. Create a signing key

Certificates, IDs & Profiles → Keys → **+**

- Enable **Sign in with Apple**, bind it to the primary App ID
- Download the `.p8` — **Apple allows this exactly once**
- Record the **Key ID** and your **Team ID**

### 4. Configure GIP

Identity Platform → Providers → Apple, on tenant `MP-Internal-e986p`:

- Services ID → `clientId`
- Team ID, Key ID, and the `.p8` contents → used to mint the client secret

### 5. Set the app env var

```
NEXT_PUBLIC_APPLE_SERVICES_ID=com.mark8ly.admin.web
```

Set it on the `mark8ly-admin` chart in `tesserix-k8s`. The button appears
once this is present (`appleSignInEnabled` in `apps/admin/lib/config.ts`).

## ⚠️ The client secret expires every 6 months

Apple caps the Sign in with Apple client secret at **6 months**. When it
lapses, web Apple sign-in breaks with no warning and no deploy to correlate
it with — it simply stops working.

**Assign an owner and a calendar reminder now.** This is the single most
common way Sign in with Apple breaks in production. Native is unaffected;
only the web flow uses this secret.

## "Hide My Email" is not solved by any of the above

If a user picks **Hide My Email**, Apple reports
`something@privaterelay.appleid.com` instead of their real address. GIP sees
a different email, so no conflict is raised and a **separate account** is
created — one that owns no tenant, and therefore no store.

This is Apple's design, not a configuration mistake, and **email-based
account linking cannot fix it**: there is no shared email to match on.

The supported path is explicit linking while already signed in:
**Settings → Security → Link Apple**, which binds the Apple identity
(relay address and all) to the existing account's UID.

The durable fix would be supporting multiple identities per owner rather
than the single `owner_user_id` the schema has today. That is a schema
change and is not planned yet.

## Implementation map

| Piece | File |
|---|---|
| Apple JS SDK loader, popup, nonce | `apps/admin/lib/gip/apple-js.ts` |
| GIP token exchange | `apps/admin/lib/gip/signup.ts` (`signInWithApple`) |
| Button + handler | `apps/admin/components/auth/SignInForm.tsx` |
| Config flag | `apps/admin/lib/config.ts` (`appleSignInEnabled`) |
| Logo | `packages/ui/src/apple-mark.tsx` |

The web flow generates a **random nonce per attempt** and binds it into the
Apple token, so the token cannot be replayed. Note that mobile currently
hardcodes an empty nonce (`apps/mobile-admin/lib/social-auth.ts`) — that is
a known gap, tracked separately; do not copy it.
