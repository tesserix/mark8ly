import { NextResponse } from "next/server";

import { config } from "@/lib/config";

// Server-side proxy for the Journal "coming soon" page's email capture
// (#153). The browser never calls marketplace-api directly — it only ever
// sees this route, which forwards to marketplace-api's public, tenant-free
// /api/v1/journal/subscribe endpoint using the server-only
// MARKETPLACE_API_URL (see lib/config.ts for why not PLATFORM_API_URL).
export const dynamic = "force-dynamic";

interface JournalSubscribeResponse {
  ok: boolean;
  message?: string;
}

function jsonResponse(body: JournalSubscribeResponse, status: number): NextResponse {
  return NextResponse.json(body, { status });
}

export async function POST(request: Request): Promise<NextResponse> {
  let email: unknown;
  try {
    const parsed = (await request.json()) as { email?: unknown };
    email = parsed.email;
  } catch {
    return jsonResponse(
      { ok: false, message: "Enter a valid email address." },
      400,
    );
  }

  if (typeof email !== "string" || email.trim() === "") {
    return jsonResponse(
      { ok: false, message: "Enter a valid email address." },
      400,
    );
  }

  let upstream: Response;
  try {
    upstream = await fetch(`${config.marketplaceApiUrl}/api/v1/journal/subscribe`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email }),
      cache: "no-store",
    });
  } catch (error: unknown) {
    // marketplace-api unreachable (network error, DNS, connection refused).
    // The visitor gets a plain, human message — never a stack trace, and
    // never a silent no-op that looks like success.
    console.error("journal-subscribe: marketplace-api unreachable", error);
    return jsonResponse(
      {
        ok: false,
        message:
          "We couldn't save that just now — please try again in a moment.",
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
    // Anything else (400 validation_failed, or a 5xx marketplace-api
    // itself couldn't recover from) reads the same to a visitor: the
    // address they typed didn't go through. marketplace-api's own logs
    // carry the real detail server-side.
    return jsonResponse(
      { ok: false, message: "That doesn't look like a valid email address." },
      upstream.status >= 500 ? 502 : 400,
    );
  }

  return jsonResponse({ ok: true }, 200);
}
