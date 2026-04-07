/**
 * auth-bff session client. Used by middleware and server components
 * to validate the session cookie against the real auth-bff service
 * rather than trusting cookie presence alone.
 *
 * This is the Phase J replacement for the Phase I "cookie presence"
 * gate. Every authenticated admin render first calls
 * `fetchSession(cookieHeader)` — if it returns null the visitor is
 * bounced to the marketing login page.
 */

const AUTH_BFF_URL = process.env.AUTH_BFF_URL ?? "http://localhost:8087";

export interface AuthSession {
  userId: string;
  email: string;
  tenantId: string;
}

interface SessionResponse {
  data: {
    user_id: string;
    email: string;
    tenant_id: string;
  };
}

/**
 * Validate the session cookie and return the resolved session.
 *
 * Forwards the raw Cookie header as-is so the auth-bff sees exactly
 * what the browser sent — including the HttpOnly session cookie we
 * can't otherwise reach from JavaScript.
 *
 * Returns `null` on:
 *   - missing cookie (auth-bff 401 "no_session")
 *   - expired or tampered cookie (auth-bff 401 "invalid_session")
 *   - auth-bff unreachable (network error)
 *
 * Callers should treat null as "please log in again" regardless of the
 * underlying cause — distinguishing them would leak information to the
 * rendered error page.
 */
export async function fetchSession(
  cookieHeader: string | null | undefined,
): Promise<AuthSession | null> {
  if (!cookieHeader) return null;

  try {
    const res = await fetch(`${AUTH_BFF_URL}/auth/session`, {
      method: "GET",
      headers: { Cookie: cookieHeader },
      // Never cache the auth probe. Every render re-validates.
      cache: "no-store",
    });

    if (!res.ok) return null;
    const body = (await res.json()) as SessionResponse;
    return {
      userId: body.data.user_id,
      email: body.data.email,
      tenantId: body.data.tenant_id,
    };
  } catch {
    // Auth-bff unreachable. Fail closed.
    return null;
  }
}

/**
 * POST to auth-bff /auth/logout to invalidate the session server-side.
 * Caller is responsible for clearing the cookie on its own origin and
 * redirecting; this function is the "tell auth-bff" half.
 */
export async function revokeSession(
  cookieHeader: string | null | undefined,
): Promise<void> {
  if (!cookieHeader) return;
  try {
    await fetch(`${AUTH_BFF_URL}/auth/logout`, {
      method: "POST",
      headers: { Cookie: cookieHeader },
      cache: "no-store",
    });
  } catch {
    // Best-effort. If auth-bff is unreachable the client still clears
    // its own cookie — the session will expire naturally.
  }
}
