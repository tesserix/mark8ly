// GET /api/locations/countries — storefront proxy for platform-api's
// /api/v1/locations/countries. Mirrors the admin proxy so client
// components (AddressBook, future checkout fieldsets) can fetch the
// country list without exposing PLATFORM_API_URL to the browser.

import { NextResponse } from "next/server";
import { listCountries } from "@/lib/api/platform-api";

export const dynamic = "force-dynamic";

export async function GET() {
  const data = await listCountries();
  return NextResponse.json(
    { data },
    { headers: { "Cache-Control": "public, max-age=3600, s-maxage=3600" } },
  );
}
