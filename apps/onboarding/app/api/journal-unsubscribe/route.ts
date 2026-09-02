import { NextResponse } from "next/server";

import { config } from "@/lib/config";

// Server-side proxy for the Journal unsubscribe page's erasure request —
// the counterpart to app/api/journal-subscribe/route.ts. The browser
// never calls marketplace-api directly — it only ever sees this route,
// which forwards to marketplace-api's public, tenant-free
// /api/v1/journal/unsubscribe endpoint using the server-only
// MARKETPLACE_API_URL (see lib/config.ts).
export const dynamic = "force-dynamic";

interface JournalUnsubscribeResponse {
  ok: boolean;
  message?: string;
}

function jsonResponse(
  body: JournalUnsubscribeResponse,
  status: number,
): NextResponse {
  return NextResponse.json(body, { status });
}

export async function POST(request: Request): Promise<NextResponse> {
  let token: unknown;
  try {
    const parsed = (await request.json()) as { token?: unknown };
    token = parsed.token;
  } catch {
    return jsonResponse(
      { ok: false, message: "That unsubscribe link looks incomplete." },
      400,
    );
  }

  if (typeof token !== "string" || token.trim() === "") {
    return jsonResponse(
      { ok: false, message: "That unsubscribe link looks incomplete." },
      400,
    );
  }

  let upstream: Response;
  try {
    upstream = await fetch(
      `${config.marketplaceApiUrl}/api/v1/journal/unsubscribe`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token }),
        cache: "no-store",
      },
    );
  } catch (error: unknown) {
    // marketplace-api unreachable (network error, DNS, connection refused).
    // The visitor gets a plain, human message — never a stack trace, and
    // never a silent no-op that looks like success.
    console.error("journal-unsubscribe: marketplace-api unreachable", error);
    return jsonResponse(
      {
        ok: false,
        message:
          "We couldn't reach the server just now — please try again in a moment.",
      },
      502,
    );
  }

  if (upstream.status === 429) {
    return jsonResponse(
      { ok: false, message: "Too many attempts. Please try again shortly." },
      429,
    );
  }

  if (!upstream.ok) {
    // marketplace-api's /journal/unsubscribe always returns 200 for a
    // syntactically valid request, whether or not the token was
    // recognised — see internal/handlers/public/journal_unsubscribe.go.
    // A non-2xx here means something actually broke upstream, not "bad
    // token", so this is the one place that's allowed to read as a real
    // failure to the visitor.
    return jsonResponse(
      { ok: false, message: "Something went wrong. Please try again." },
      upstream.status >= 500 ? 502 : 400,
    );
  }

  return jsonResponse({ ok: true }, 200);
}
