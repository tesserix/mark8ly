// Server-side proxy to tesserix-home's /api/internal/platform-announcements.
// The browser hits THIS route (no token), the route forwards with the
// shared INTERNAL_API_TOKEN to tesserix. Keeping the proxy here means the
// token never leaves the pod and the banner client component can stay
// simple (plain fetch from same origin, credentials inherited).

import { NextResponse, type NextRequest } from "next/server";

const TESSERIX_INTERNAL_URL =
  process.env.TESSERIX_INTERNAL_URL ?? "https://tesserix.app";
const INTERNAL_API_TOKEN = process.env.INTERNAL_API_TOKEN ?? "";

export async function GET(_req: NextRequest) {
  if (!INTERNAL_API_TOKEN) {
    return NextResponse.json({ rows: [] });
  }
  try {
    const url = new URL(
      `${TESSERIX_INTERNAL_URL}/api/internal/platform-announcements`,
    );
    url.searchParams.set("product", "mark8ly");
    url.searchParams.set("tenant_status", "active");

    const res = await fetch(url.toString(), {
      headers: { Authorization: `Bearer ${INTERNAL_API_TOKEN}` },
      cache: "no-store",
    });
    if (!res.ok) {
      return NextResponse.json({ rows: [] });
    }
    const body = await res.json();
    return NextResponse.json(body);
  } catch {
    return NextResponse.json({ rows: [] });
  }
}
