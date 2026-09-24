/**
 * The failure this guards is silence: a crash reporter that is installed,
 * looks configured, and reports nothing. That shape has cost this estate a
 * day more than once — a Stripe key with a trailing newline, a metrics
 * endpoint nothing could scrape, 1720 tests no workflow ran.
 */
const mockInit = jest.fn()
const mockCapture = jest.fn()
jest.mock('@sentry/react-native', () => ({
  init: (...a: unknown[]) => mockInit(...a),
  captureException: (...a: unknown[]) => mockCapture(...a),
}))

const load = () => {
  let mod!: typeof import('../lib/crash-reporting')
  jest.isolateModules(() => {
    mod = require('../lib/crash-reporting')
  })
  return mod
}

beforeEach(() => {
  mockInit.mockClear()
  mockCapture.mockClear()
  delete process.env.EXPO_PUBLIC_SENTRY_DSN
})

describe('without a DSN', () => {
  it('does not initialise and does not report', () => {
    const m = load()
    expect(m.crashReportingEnabled).toBe(false)
    m.initCrashReporting()
    m.captureException(new Error('boom'))
    // Dev, the demo build and all 132 suites run with no DSN. None of them
    // should open a network connection or need a Sentry project to exist.
    expect(mockInit).not.toHaveBeenCalled()
    expect(mockCapture).not.toHaveBeenCalled()
  })

  it('treats whitespace as absent rather than as a DSN', () => {
    process.env.EXPO_PUBLIC_SENTRY_DSN = '   \n'
    expect(load().crashReportingEnabled).toBe(false)
  })
})

describe('with a DSN', () => {
  it('initialises with it', () => {
    process.env.EXPO_PUBLIC_SENTRY_DSN = 'https://abc@o1.ingest.sentry.io/123'
    const m = load()
    expect(m.crashReportingEnabled).toBe(true)
    m.initCrashReporting()
    expect(mockInit).toHaveBeenCalledTimes(1)
    expect(mockInit.mock.calls[0][0]).toMatchObject({
      dsn: 'https://abc@o1.ingest.sentry.io/123',
    })
  })

  /**
   * The Stripe key was read from Secret Manager with a trailing newline,
   * nothing trimmed it, and every call failed at the Authorization header
   * while the secret looked correct in the dashboard. EXPO_PUBLIC_* values
   * come from eas.json and CI and can carry the same whitespace.
   */
  it('trims a DSN that arrives with a trailing newline', () => {
    process.env.EXPO_PUBLIC_SENTRY_DSN = 'https://abc@o1.ingest.sentry.io/123\n'
    load().initCrashReporting()
    expect(mockInit.mock.calls[0][0].dsn).toBe('https://abc@o1.ingest.sentry.io/123')
  })

  it('does not ship merchant PII to a third party', () => {
    process.env.EXPO_PUBLIC_SENTRY_DSN = 'https://abc@o1.ingest.sentry.io/123'
    load().initCrashReporting()
    // Orders, customers and payouts pass through every screen in this app.
    expect(mockInit.mock.calls[0][0].sendDefaultPii).toBe(false)
  })

  it('reports handled errors with a context tag', () => {
    process.env.EXPO_PUBLIC_SENTRY_DSN = 'https://abc@o1.ingest.sentry.io/123'
    const err = new Error('boom')
    load().captureException(err, 'render-error-boundary')
    expect(mockCapture).toHaveBeenCalledWith(err, {
      tags: { context: 'render-error-boundary' },
    })
  })
})
