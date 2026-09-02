// Browser-side helper for the Journal unsubscribe page (erasure
// counterpart to #153's email capture — see lib/api/journal-signup.ts).
// Calls this app's own /api/journal-unsubscribe route — never
// marketplace-api directly, for the same reason subscribeToJournal
// doesn't: only a server route is allowed to hold MARKETPLACE_API_URL.

export type UnsubscribeResult = { ok: true } | { ok: false; message: string };

/**
 * Submits an unsubscribe token. Never throws — network failures and
 * non-2xx responses both come back as `{ ok: false, message }` so the
 * caller can render the error inline without a try/catch.
 *
 * A 200 here does not mean the token was recognised — marketplace-api's
 * /journal/unsubscribe deliberately returns 200 for an unknown, already-
 * used, or malformed token too, so a bearer-token guess can't be used to
 * enumerate valid addresses. This function just forwards that response.
 */
export async function unsubscribeFromJournal(
  token: string,
): Promise<UnsubscribeResult> {
  try {
    const res = await fetch("/api/journal-unsubscribe", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token }),
    });

    const body = (await res.json().catch(() => ({}))) as { message?: string };

    if (!res.ok) {
      return {
        ok: false,
        message: body.message ?? "Something went wrong. Please try again.",
      };
    }

    return { ok: true };
  } catch {
    // fetch() itself rejected — offline, or the app server is unreachable.
    return {
      ok: false,
      message:
        "We couldn't reach the server. Check your connection and try again.",
    };
  }
}
