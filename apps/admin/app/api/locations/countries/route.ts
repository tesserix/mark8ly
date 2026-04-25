// GET /api/locations/countries — admin proxy for platform-api's
// /api/v1/locations/countries. Exists so client components (e.g.
// AddressFieldset) can fetch the country list without a browser-visible
// PLATFORM_API_URL. Reference data, no auth needed downstream.

import { NextResponse } from "next/server";
import { listCountries } from "@/lib/api/platform-api";

export const dynamic = "force-dynamic";

export async function GET() {
  const data = await listCountries();
  return NextResponse.json(
    { data },
    {
      // Country list is seed data; safe to cache hard at the edge.
      headers: { "Cache-Control": "public, max-age=3600, s-maxage=3600" },
    },
  );
}
