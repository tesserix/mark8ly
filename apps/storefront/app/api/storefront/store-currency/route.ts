import { NextResponse } from "next/server";
import { headers } from "next/headers";
import { fetchBranding } from "@/lib/api/marketplace-api";
import { resolveStoreSlug } from "@/lib/slug";

// Returns the store's currency code as resolved from the host. Used by
// server components that need to know the currency before rendering
// the gift card purchase form.
export const dynamic = "force-dynamic";

export async function GET() {
  const h = await headers();
  const host = h.get("host");
  const storeSlug =
    await resolveStoreSlug(host);

  const branding = await fetchBranding(storeSlug);
  const currency =
    branding?.store?.currency_code ??
    process.env.DEFAULT_STORE_CURRENCY ??
    "INR";

  return NextResponse.json({ currency });
}
