/**
 * Apple App Site Association — the document iOS fetches to decide whether a
 * link to admin.mark8ly.com may open the native admin app.
 *
 * app.config.js has declared `associatedDomains: ['applinks:admin.mark8ly.com']`
 * for the whole life of the iOS app, and this URL returned 404. A declared
 * associated domain with no document is not a partial feature: iOS silently
 * declines to register the domain, so every universal link falls through to
 * Safari and the capability the app advertises has never worked once.
 *
 * Apple's requirements this route exists to satisfy:
 *   - served over https at /.well-known/apple-app-site-association
 *   - no file extension, and `application/json`
 *   - 200 with NO redirect — a 30x is a failure, which is why the admin
 *     middleware matcher excludes .well-known rather than this path being
 *     added to PUBLIC_PREFIXES: that list is consulted after several
 *     canonical-host branches that can redirect or 404 first.
 *
 * WHAT IS CLAIMED, AND WHY IT IS AN ALLOWLIST
 *
 * Only the screens the app can actually render. This deliberately does NOT
 * mirror Android's `pathPrefix: '/'`, which claims the entire host.
 *
 * Claiming everything would break flows that arrive by email. /reset-password
 * and /accept-invite are landed on from a link in a message: if the app
 * claimed them, a merchant tapping that link on an iPhone with the app
 * installed would be handed to an app that has no reset or invite screen, and
 * could not complete either flow. /login, /auth/* and /logout are likewise
 * web-only — the native app authenticates through its own `mark8ly-admin://`
 * scheme (see app/login.tsx IDP_REDIRECT_URL), so universal links are not
 * part of sign-in at all and claiming those paths would buy nothing.
 *
 * Each entry is paired: the bare path and its subtree, because "/orders"
 * does not match "/orders/123" on its own.
 */
import { NextResponse } from 'next/server'

// Team ID from eas.json `appleTeamId`, bundle ID from app.config.js.
const APP_ID = '2CRHRRYBPL.com.mark8ly.admin'

// Web routes under app/(admin)/ that have a counterpart screen in
// apps/mobile-admin/app/(tabs)/. The route group "(admin)" is not part of
// the URL, so these are the real paths.
const CLAIMED_SECTIONS = [
  'dashboard',
  'orders',
  'products',
  'customers',
  'marketing',
  'settings',
  'support',
] as const

const AASA = {
  applinks: {
    details: [
      {
        appIDs: [APP_ID],
        components: CLAIMED_SECTIONS.flatMap((section) => [
          { '/': `/${section}` },
          { '/': `/${section}/*` },
        ]),
      },
    ],
  },
  // The app uses neither of these. Declaring them empty is how Apple's
  // validator reads "deliberately none" rather than "author forgot".
  webcredentials: { apps: [] as string[] },
  appclips: { apps: [] as string[] },
}

// Static: the document depends on nothing per-request, and iOS fetches it
// through Apple's CDN rather than from the device.
export const dynamic = 'force-static'

export function GET() {
  return new NextResponse(JSON.stringify(AASA, null, 2), {
    status: 200,
    headers: {
      'content-type': 'application/json',
      'cache-control': 'public, max-age=3600',
    },
  })
}
