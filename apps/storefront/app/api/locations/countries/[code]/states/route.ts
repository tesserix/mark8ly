// GET /api/locations/countries/[code]/states — storefront proxy for
// platform-api's per-country state list. Used by AddressBook's
// dependent State/Region dropdown.

import { NextResponse } from "next/server";
import { listStatesByCountry } from "@/lib/api/platform-api";

export const dynamic = "force-dynamic";

export async function GET(
  _req: Request,
  ctx: { params: Promise<{ code: string }> },
) {
  const { code } = await ctx.params;
  const data = await listStatesByCountry(code);
  return NextResponse.json(
    { data },
    { headers: { "Cache-Control": "public, max-age=3600, s-maxage=3600" } },
  );
}
