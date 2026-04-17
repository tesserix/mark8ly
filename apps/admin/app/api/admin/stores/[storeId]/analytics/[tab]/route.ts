// Same-origin proxy for client-side analytics fetches. The AnalyticsSection
// component switches tabs and ranges without a full page reload, so it
// calls this route instead of marketplace-api directly — session headers
// are injected here from the middleware-set x-session-* headers.

import { headers } from "next/headers";
import { NextResponse } from "next/server";

import {
  fetchCustomersMetrics,
  fetchOrdersMetrics,
  fetchReviewsMetrics,
  fetchSalesMetrics,
  type AnalyticsRange,
  type MetricsTab,
} from "@/lib/api/marketplace-api";

const VALID_TABS: readonly MetricsTab[] = [
  "sales",
  "orders",
  "customers",
  "reviews",
];
const VALID_RANGES: readonly AnalyticsRange[] = ["7d", "30d", "90d"];

export async function GET(
  request: Request,
  { params }: { params: Promise<{ storeId: string; tab: string }> },
): Promise<Response> {
  const { storeId, tab } = await params;
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";

  if (!userId || !tenantId) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }

  if (!VALID_TABS.includes(tab as MetricsTab)) {
    return NextResponse.json({ error: "invalid_tab" }, { status: 400 });
  }

  const url = new URL(request.url);
  const rangeParam = (url.searchParams.get("range") ?? "30d") as AnalyticsRange;
  if (!VALID_RANGES.includes(rangeParam)) {
    return NextResponse.json({ error: "invalid_range" }, { status: 400 });
  }

  const session = { userId, tenantId };
  const result =
    tab === "sales"
      ? await fetchSalesMetrics(storeId, rangeParam, session)
      : tab === "orders"
        ? await fetchOrdersMetrics(storeId, rangeParam, session)
        : tab === "customers"
          ? await fetchCustomersMetrics(storeId, rangeParam, session)
          : await fetchReviewsMetrics(storeId, rangeParam, session);

  if (result === null) {
    return NextResponse.json({ error: "not_found" }, { status: 404 });
  }
  return NextResponse.json(result);
}
