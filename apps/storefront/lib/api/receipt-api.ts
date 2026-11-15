// Server-side only: it sends the storefront key.
import type { Receipt } from "@/lib/receipt";

const MARKETPLACE_API_URL = process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";

function receiptUrl(slug: string, token: string): string {
  return `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(slug)}/receipts/${encodeURIComponent(token)}`;
}

function headers(): HeadersInit {
  return STOREFRONT_KEY
    ? { Accept: "application/json", "X-Storefront-Key": STOREFRONT_KEY }
    : { Accept: "application/json" };
}

// null for an unknown token or any failure: the page shows not-found.
export async function fetchReceipt(slug: string, token: string): Promise<Receipt | null> {
  try {
    const res = await fetch(receiptUrl(slug, token), { headers: headers(), cache: "no-store" });
    return res.ok ? ((await res.json()) as Receipt) : null;
  } catch {
    return null;
  }
}

export async function resendReceipt(slug: string, token: string): Promise<boolean> {
  try {
    const res = await fetch(`${receiptUrl(slug, token)}/resend`, { method: "POST", headers: headers(), cache: "no-store" });
    return res.status === 202;
  } catch {
    return false;
  }
}
