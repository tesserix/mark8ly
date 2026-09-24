/**
 * The app has declared `applinks:admin.mark8ly.com` since it shipped while
 * this URL returned 404, so iOS never registered the domain and no universal
 * link has ever opened the app. These assertions are about the two ways that
 * silently comes back: the document becoming unreachable, or the claim
 * widening to swallow links that must stay in the browser.
 */
import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { GET } from './route'

async function body() {
  return JSON.parse(await GET().text())
}

describe('apple-app-site-association', () => {
  it('is served as application/json with no redirect', async () => {
    const res = GET()
    // Apple rejects any 30x and requires the JSON content type.
    expect(res.status).toBe(200)
    expect(res.headers.get('content-type')).toBe('application/json')
  })

  it('carries the team-prefixed bundle id', async () => {
    const aasa = await body()
    expect(aasa.applinks.details[0].appIDs).toEqual([
      '2CRHRRYBPL.com.mark8ly.admin',
    ])
  })

  it('claims each section as both the bare path and its subtree', async () => {
    const paths = (await body()).applinks.details[0].components.map(
      (c: Record<string, string>) => c['/'],
    )
    // "/orders" alone does not match "/orders/123".
    for (const section of ['dashboard', 'orders', 'products', 'customers']) {
      expect(paths).toContain(`/${section}`)
      expect(paths).toContain(`/${section}/*`)
    }
  })

  // The regression that would hurt merchants rather than merely not help
  // them: these arrive as links in email, and the app has no screen for any
  // of them. Claiming them strands the tap in an app that cannot continue.
  it('does not claim web-only or email-landing flows', async () => {
    const paths: string[] = (await body()).applinks.details[0].components.map(
      (c: Record<string, string>) => c['/'],
    )
    for (const web of [
      '/reset-password',
      '/accept-invite',
      '/forgot-password',
      '/login',
      '/logout',
      '/pick-tenant',
      '/auth',
      '/api',
      '/webhooks',
    ]) {
      expect(
        paths.some((p) => p === web || p === `${web}/*` || p === '*' || p === '/*'),
      ).toBe(false)
    }
  })

  // Serving the right document is useless if middleware redirects the
  // request before it is reached.
  it('is excluded from the middleware matcher', () => {
    const mw = readFileSync(join(__dirname, '..', '..', '..', 'middleware.ts'), 'utf8')
    const matcher = /matcher:\s*\[([^\]]*)\]/.exec(mw)?.[1] ?? ''
    expect(matcher).toContain('well-known')
  })
})
