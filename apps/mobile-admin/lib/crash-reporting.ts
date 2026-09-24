/**
 * Crash reporting for mobile-admin.
 *
 * A crash on a merchant's phone reaches none of the server-side stack. The
 * Go services ship to Cloud Logging with 30-day retention, Prometheus alerts
 * on error rate, and traces land in ClickHouse — a device does not appear in
 * any of it. Apple's own crash reports are also much weaker here than for a
 * native app: they symbolicate NATIVE frames, so a JavaScript error that
 * brings down React Native yields bridge and runtime frames rather than the
 * screen or handler that actually failed.
 *
 * Everything goes through this module so the ErrorBoundary keeps the single
 * telemetry seam it was written with ("wire the capture call HERE and nowhere
 * else"). Nothing else should import @sentry/react-native directly.
 */
import * as Sentry from '@sentry/react-native'

/**
 * The DSN is trimmed, which is not defensive noise.
 *
 * EXPO_PUBLIC_* values arrive from eas.json and CI, and this estate has
 * already lost a day to exactly this: the Stripe key was read from GCP Secret
 * Manager with a trailing newline, envconfig did not trim it, and every
 * Stripe call failed at the Authorization header while the secret looked
 * perfectly correct in the dashboard. A DSN with a stray newline fails the
 * same way — Sentry.init rejects it and reports nothing, silently.
 */
const DSN = (process.env.EXPO_PUBLIC_SENTRY_DSN ?? '').trim()

/**
 * Absent DSN is a supported state, not a misconfiguration.
 *
 * Development, the demo build and all 132 jest suites run without one, and
 * none of them should open a network connection or need a Sentry project to
 * exist. A build that ships without the variable set simply reports nothing,
 * which is the behaviour before this module existed.
 */
export const crashReportingEnabled = DSN !== ''

export function initCrashReporting(): void {
  if (!crashReportingEnabled) return

  Sentry.init({
    dsn: DSN,
    // Distinguishes a TestFlight build's crashes from production's.
    environment: process.env.EXPO_PUBLIC_SENTRY_ENVIRONMENT?.trim() || 'production',

    // Merchant data passes through every screen in this app: orders,
    // customers, payouts. sendDefaultPii would attach request headers,
    // cookies and user identifiers to every event and ship them to a third
    // party. The repo runs a PII leak guard over added log lines for the
    // same reason; a crash reporter is the same exposure with a longer
    // retention.
    sendDefaultPii: false,

    // Native crashes are the half Apple's Organizer already covers. The
    // reason this SDK is here is the JS half, so both are on.
    enableNativeCrashHandling: true,

    // No performance tracing. It samples every navigation and is a separate
    // decision with its own cost; turning it on by default would quietly
    // change what this ships.
    tracesSampleRate: 0,
  })
}

/**
 * Report a handled error. No-ops when no DSN is configured.
 *
 * `context` is a short, non-PII label for where the failure came from —
 * a screen or subsystem name, never merchant data.
 */
export function captureException(error: unknown, context?: string): void {
  if (!crashReportingEnabled) return
  Sentry.captureException(error, context ? { tags: { context } } : undefined)
}
