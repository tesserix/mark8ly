// marketplace-api calls for the in-store sale screen (#1105). Server-only:
// MARKETPLACE_API_URL is in-cluster, so the sale screen reaches these
// through the server actions in app/(admin)/sell/actions.ts.

import {
  MARKETPLACE_API_URL,
  type MutationResult,
  type SessionHeaders,
  commonHeaders,
  parseMutationError,
} from "./marketplace-api";

export interface InStoreVariant {
  id: string;
  option_summary?: string;
  price: string;
  currency_code: string;
  available: number;
  inventory_policy: "deny" | "continue" | string;
}

export interface InStoreProduct {
  id: string;
  title: string;
  image_url?: string;
  variants: InStoreVariant[];
}

export interface InStoreSaleLine {
  variant_id?: string;
  custom_title?: string;
  custom_price?: string;
  quantity: number;
}

export interface InStoreQuoteBody {
  lines: InStoreSaleLine[];
  discount?: string;
}

export interface InStoreSaleBody extends InStoreQuoteBody {
  idempotency_key: string;
  payment_method: string;
  payment_method_label?: string;
  cash_tendered?: string;
  note?: string;
  customer_email?: string;
  expected_total: string;
}

export interface InStoreQuote {
  subtotal: string;
  tax_total: string;
  discount_total: string;
  grand_total: string;
  currency_code: string;
}

// "unreachable" means the outcome is unknown (no answer, or a 5xx): the
// sale may have been recorded, so the caller retries with the same key.
// "rejected" is any other 4xx: definitely not recorded.
export type SaleResult =
  | { ok: true; orderId: string; orderNumber: string; grandTotal: string; reused: boolean }
  | { ok: false; kind: "out_of_stock"; variantId: string }
  | { ok: false; kind: "total_changed"; grandTotal: string }
  | { ok: false; kind: "rejected"; message: string }
  | { ok: false; kind: "unreachable" };

const base = (storeId: string) => `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/in-store`;

export async function getInStoreCatalog(storeId: string, session: SessionHeaders): Promise<InStoreProduct[]> {
  const res = await fetch(`${base(storeId)}/catalog`, { cache: "no-store", headers: commonHeaders(session) });
  if (!res.ok) return [];
  const body = (await res.json()) as { data?: InStoreProduct[] } | InStoreProduct[];
  return Array.isArray(body) ? body : (body.data ?? []);
}

export async function quoteInStoreSale(
  storeId: string,
  body: InStoreQuoteBody,
  session: SessionHeaders,
): Promise<MutationResult<InStoreQuote>> {
  const res = await fetch(`${base(storeId)}/quote`, {
    method: "POST",
    cache: "no-store",
    headers: commonHeaders(session),
    body: JSON.stringify(body),
  });
  if (res.ok) return { ok: true, data: (await res.json()) as InStoreQuote };
  return { ok: false, error: await parseMutationError(res) };
}

export async function recordInStoreSale(
  storeId: string,
  body: InStoreSaleBody,
  session: SessionHeaders,
): Promise<SaleResult> {
  let res: Response;
  try {
    res = await fetch(`${base(storeId)}/sales`, {
      method: "POST",
      cache: "no-store",
      headers: commonHeaders(session),
      body: JSON.stringify(body),
    });
  } catch {
    return { ok: false, kind: "unreachable" };
  }
  if (res.status >= 500) return { ok: false, kind: "unreachable" };

  const json = (await res.json().catch(() => null)) as Record<string, unknown> | null;
  if (res.ok) {
    return {
      ok: true,
      orderId: String(json?.id ?? ""),
      orderNumber: String(json?.order_number ?? ""),
      grandTotal: String(json?.grand_total ?? ""),
      reused: res.status === 200,
    };
  }
  if (json?.error === "out_of_stock") return { ok: false, kind: "out_of_stock", variantId: String(json.variant_id ?? "") };
  if (json?.error === "total_changed") return { ok: false, kind: "total_changed", grandTotal: String(json.grand_total ?? "") };
  return { ok: false, kind: "rejected", message: String(json?.message ?? `The sale was refused (${res.status}).`) };
}
