// Browser-side helper for the Journal email capture form (#153). Calls
// this app's own /api/journal-subscribe route — never marketplace-api
// directly, since that route is the only thing allowed to hold
// MARKETPLACE_API_URL.

export type SubscribeResult = { ok: true } | { ok: false; message: string };

/**
 * Submits an email to the Journal "coming soon" capture point. Never
 * throws — network failures and non-2xx responses both come back as
 * `{ ok: false, message }` so the caller can render the error inline
 * without a try/catch.
 */
export async function subscribeToJournal(email: string): Promise<SubscribeResult> {
  try {
    const res = await fetch("/api/journal-subscribe", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email }),
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
