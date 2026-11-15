"use server";

import { headers } from "next/headers";

import { resendReceipt } from "@/lib/api/receipt-api";
import { resolveStoreSlug } from "@/lib/slug";

// The store comes from the address the page was opened on, never the client.
export async function requestNewReceiptLink(token: string): Promise<boolean> {
  const slug = await resolveStoreSlug((await headers()).get("host"));
  if (!slug) return false;
  return resendReceipt(slug, token);
}
