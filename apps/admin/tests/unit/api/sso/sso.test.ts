/**
 * Tenant SSO configuration client (#820).
 *
 * Two behaviours carry real consequences and neither is visible from the
 * types, so they are pinned here:
 *
 *   1. The redaction placeholder must never be submitted. The server returns
 *      "[redacted]" in place of a saved secret reference; sending it back
 *      stores that literal string and leaves a configuration that looks
 *      correct on screen and cannot resolve a secret.
 *   2. A failed connection test is an ANSWER (422 with a body), not a
 *      transport failure. Throwing it away would leave the merchant with a
 *      generic error instead of the reason.
 */
import { describe, it, expect, beforeAll, afterEach, afterAll, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'

import {
  getSSOConfig,
  saveSSOConfig,
  testSSOConnection,
} from '@/lib/api/sso/sso'
import { REDACTED } from '@/lib/api/sso/schemas'

const server = setupServer()
beforeAll(() => server.listen({ onUnhandledRequest: 'error' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

const CONFIG = '/api/admin/sso/config'
const TEST = '/api/admin/sso/test'

function savedConfig(overrides: Record<string, unknown> = {}) {
  return {
    tenant_id: 'f47ac10b-58cc-4372-a567-0e02b2c3d479',
    provider: 'oidc',
    metadata: {
      issuer: 'https://accounts.example.com',
      client_id: 'client-abc',
      client_secret_ref: REDACTED,
      redirect_uri: '',
      scopes: '',
    },
    attr_mapping: {},
    enabled: true,
    ...overrides,
  }
}

describe('getSSOConfig', () => {
  it('parses a saved configuration', async () => {
    server.use(http.get(CONFIG, () => HttpResponse.json(savedConfig())))

    const cfg = await getSSOConfig()
    expect(cfg?.metadata.issuer).toBe('https://accounts.example.com')
    expect(cfg?.enabled).toBe(true)
  })

  // Not an error: no SSO configured is the normal state for every tenant that
  // has not set it up, and the screen renders an empty form for it.
  it('returns null when no configuration exists', async () => {
    server.use(
      http.get(CONFIG, () =>
        HttpResponse.json({ error: 'not_found' }, { status: 404 }),
      ),
    )
    await expect(getSSOConfig()).resolves.toBeNull()
  })

  it('still throws on a real failure', async () => {
    server.use(
      http.get(CONFIG, () =>
        HttpResponse.json({ error: 'internal_error' }, { status: 500 }),
      ),
    )
    await expect(getSSOConfig()).rejects.toBeTruthy()
  })
})

describe('saveSSOConfig', () => {
  // The bug this prevents: storing the literal string "[redacted]" as the
  // secret path, producing a config that reads correctly and can never
  // resolve a secret.
  it('omits the secret reference when it is still the redaction placeholder', async () => {
    const seen = vi.fn()
    server.use(
      http.post(CONFIG, async ({ request }) => {
        seen(await request.json())
        return HttpResponse.json({}, { status: 201 })
      }),
    )

    await saveSSOConfig({
      provider: 'oidc',
      metadata: {
        issuer: 'https://accounts.example.com',
        client_id: 'client-abc',
        client_secret_ref: REDACTED,
      },
      enabled: true,
    })

    const body = seen.mock.calls[0][0] as { metadata: Record<string, string> }
    expect(body.metadata).not.toHaveProperty('client_secret_ref')
    expect(body.metadata.issuer).toBe('https://accounts.example.com')
  })

  it('omits it when left blank, so an untouched field keeps the saved value', async () => {
    const seen = vi.fn()
    server.use(
      http.post(CONFIG, async ({ request }) => {
        seen(await request.json())
        return HttpResponse.json({}, { status: 201 })
      }),
    )

    await saveSSOConfig({
      provider: 'oidc',
      metadata: { issuer: 'https://x.example', client_id: 'c', client_secret_ref: '   ' },
      enabled: false,
    })

    const body = seen.mock.calls[0][0] as { metadata: Record<string, string> }
    expect(body.metadata).not.toHaveProperty('client_secret_ref')
  })

  it('sends a genuinely new reference', async () => {
    const seen = vi.fn()
    server.use(
      http.post(CONFIG, async ({ request }) => {
        seen(await request.json())
        return HttpResponse.json({}, { status: 201 })
      }),
    )

    await saveSSOConfig({
      provider: 'oidc',
      metadata: {
        issuer: 'https://x.example',
        client_id: 'c',
        client_secret_ref: 'kv/acme/idp',
      },
      enabled: true,
    })

    const body = seen.mock.calls[0][0] as { metadata: Record<string, string> }
    expect(body.metadata.client_secret_ref).toBe('kv/acme/idp')
  })

  it('omits the optional fields rather than sending empty strings', async () => {
    const seen = vi.fn()
    server.use(
      http.post(CONFIG, async ({ request }) => {
        seen(await request.json())
        return HttpResponse.json({}, { status: 201 })
      }),
    )

    await saveSSOConfig({
      provider: 'oidc',
      metadata: {
        issuer: 'https://x.example',
        client_id: 'c',
        client_secret_ref: 'kv/a',
        redirect_uri: '',
        scopes: '',
      },
      enabled: true,
    })

    const body = seen.mock.calls[0][0] as { metadata: Record<string, string> }
    expect(body.metadata).not.toHaveProperty('redirect_uri')
    expect(body.metadata).not.toHaveProperty('scopes')
  })
})

describe('testSSOConnection', () => {
  // The distinction the copy depends on: "we reached your IdP" and "the shape
  // looks right" are different claims, and only one of them means a merchant
  // can sign in.
  it('reports that the identity provider was reached', async () => {
    server.use(
      http.post(TEST, () =>
        HttpResponse.json({ ok: true, provider: 'oidc', checked: 'idp' }),
      ),
    )
    const res = await testSSOConnection()
    expect(res.ok).toBe(true)
    expect(res.checked).toBe('idp')
  })

  it('reports a shape-only pass distinctly', async () => {
    server.use(
      http.post(TEST, () =>
        HttpResponse.json({ ok: true, provider: 'oidc', checked: 'configuration' }),
      ),
    )
    expect((await testSSOConnection()).checked).toBe('configuration')
  })

  // A refusal is an answer. Throwing it would replace the reason the merchant
  // needs with a generic failure.
  it('returns the reason from a 422 instead of throwing', async () => {
    server.use(
      http.post(TEST, () =>
        HttpResponse.json(
          { ok: false, provider: 'oidc', error: 'we could not read the client secret' },
          { status: 422 },
        ),
      ),
    )

    const res = await testSSOConnection()
    expect(res.ok).toBe(false)
    expect(res.error).toContain('client secret')
  })

  it('throws when the test could not be run at all', async () => {
    server.use(
      http.post(TEST, () => HttpResponse.json({ error: 'boom' }, { status: 500 })),
    )
    await expect(testSSOConnection()).rejects.toBeTruthy()
  })
})
