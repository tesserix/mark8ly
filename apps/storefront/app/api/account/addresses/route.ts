// Proxy for the authenticated customer address book (list + create).

import { cookies, headers } from "next/headers";
import { resolveStoreSlug } from "@/lib/slug";
import { marketplaceStoreUrl, proxyJson } from "../../checkout/_proxy";

export const dynamic = "force-dynamic";

async function resolveSlug(): Promise<string | null> {
  const h = await headers();
  return await resolveStoreSlug(h.get("host"));
}

async function sessionHeaders(): Promise<Record<string, string>> {
  const cookieStore = await cookies();
  const session = cookieStore.get("mp_customer_session")?.value;
  const out: Record<string, string> = {};
  if (session) out.Cookie = `mp_customer_session=${session}`;
  return out;
}

export async function GET(): Promise<Response> {
  const slug = await resolveSlug();
  if (!slug) return Response.json({ error: "missing_store" }, { status: 400 });
  return proxyJson(`${marketplaceStoreUrl(slug)}/account/addresses`, {
    headers: await sessionHeaders(),
  });
}

export async function POST(req: Request): Promise<Response> {
  const slug = await resolveSlug();
  if (!slug) return Response.json({ error: "missing_store" }, { status: 400 });
  const body = await req.text();
  return proxyJson(`${marketplaceStoreUrl(slug)}/account/addresses`, {
    method: "POST",
    body,
    headers: await sessionHeaders(),
  });
}
