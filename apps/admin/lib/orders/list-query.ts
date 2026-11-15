// The Orders list filters live in the URL. One builder keeps every filter
// when another changes, so a picked chip never drops the rest.

import type { ListOrdersQuery, OrderChannel, OrderStatus, PaymentStatus } from "@/lib/api/marketplace-api";

const ORDER_STATUSES: readonly OrderStatus[] = ["pending", "confirmed", "fulfilled", "cancelled"];
const PAYMENT_STATUSES: readonly PaymentStatus[] = [
  "pending",
  "authorized",
  "paid",
  "failed",
  "refunded",
  "partially_refunded",
];
const CHANNELS: readonly OrderChannel[] = ["online", "in_store"];

function oneOf<T extends string>(allowed: readonly T[], v: unknown): T | undefined {
  return typeof v === "string" && (allowed as readonly string[]).includes(v) ? (v as T) : undefined;
}

export function parseOrdersQuery(raw: Record<string, string | string[] | undefined>): ListOrdersQuery {
  const page = typeof raw.page === "string" ? Number.parseInt(raw.page, 10) : undefined;
  const pageSize = typeof raw.page_size === "string" ? Number.parseInt(raw.page_size, 10) : undefined;
  const search = typeof raw.search === "string" ? raw.search.trim() : "";
  return {
    status: oneOf(ORDER_STATUSES, raw.status),
    paymentStatus: oneOf(PAYMENT_STATUSES, raw.payment_status),
    channel: oneOf(CHANNELS, raw.channel),
    search: search || undefined,
    page: page && page > 0 ? page : undefined,
    pageSize: pageSize && pageSize > 0 && pageSize <= 200 ? pageSize : undefined,
  };
}

// Changing anything but the page starts again at page 1.
export function ordersHref(q: ListOrdersQuery, change: Partial<ListOrdersQuery>): string {
  const next = { ...q, ...change };
  if (!("page" in change)) next.page = undefined;
  const p = new URLSearchParams();
  if (next.status) p.set("status", next.status);
  if (next.paymentStatus) p.set("payment_status", next.paymentStatus);
  if (next.channel) p.set("channel", next.channel);
  if (next.search) p.set("search", next.search);
  if (next.pageSize) p.set("page_size", String(next.pageSize));
  if (next.page && next.page > 1) p.set("page", String(next.page));
  const qs = p.toString();
  return qs ? `/orders?${qs}` : "/orders";
}
