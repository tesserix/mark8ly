// Server-side proxy for the announcements banner.
//
// The browser hits THIS route with no credentials; the route calls
// tesserix-home's platform API with a Zitadel machine token minted in-pod. The
// token never leaves the pod and the banner client component stays a plain
// same-origin fetch.
//
// # Repointed from apps/web (tesserix-home#152)
//
// This used to call `/api/internal/platform-announcements` with the shared
// INTERNAL_API_TOKEN. It now calls `/v1/announcements`, which takes the
// PRODUCT from the scope the machine token resolves to rather than from a
// query parameter — so this proxy can no longer ask about another product's
// audience, and sending `?product=` would be a 400 rather than a no-op.
//
// # The browser contract is unchanged
//
// The banner reads `{ rows }`, and it still does. The platform API answers a
// §4.4 envelope (`{ data: { announcements } }`), which is unwrapped here rather
// than passed through — forwarding the envelope would leave the banner reading
// an absent `rows` and rendering "no announcements" instead of erroring.

import { NextResponse, type NextRequest } from "next/server";

import { getPlatformToken } from "@/lib/api/platform-token";

const TESSERIX_PLATFORM_API_URL =
  process.env.TESSERIX_PLATFORM_API_URL ?? "http://platform-api.tesserix.svc.cluster.local";

// The lifecycle status the banner asks on behalf of.
//
// Hard-coded `active`, exactly as before the repoint: this proxy serves the
// admin shell's banner, and a merchant reading it is in an active tenant by
// definition. A suspended tenant does not reach this page.
const TENANT_STATUS = "active";

/** An empty banner. Never an error — see the comment on the catch below. */
function empty() {
  return NextResponse.json({ rows: [] });
}

export async function GET(_req: NextRequest) {
  try {
    const url = new URL(`${TESSERIX_PLATFORM_API_URL}/v1/announcements`);
    url.searchParams.set("tenant_status", TENANT_STATUS);

    const res = await fetch(url.toString(), {
      headers: { Authorization: `Bearer ${await getPlatformToken()}` },
      cache: "no-store",
    });
    if (!res.ok) return empty();

    const body = (await res.json()) as { data?: { announcements?: unknown[] } };
    return NextResponse.json({ rows: body.data?.announcements ?? [] });
  } catch {
    // A banner is decoration on someone's dashboard. It must never be the
    // reason the page fails, so every failure — an unmintable token included —
    // degrades to no banner rather than propagating.
    return empty();
  }
}
