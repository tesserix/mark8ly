// Proxy for individual address mutations.

import { cookies, headers } from "next/headers";
import { slugFromHost } from "@/lib/slug";
import { marketplaceStoreUrl, proxyJson } from "../../../checkout/_proxy";

export const dynamic = "force-dynamic";

async function resolveSlug(): Promise<string | null> {
  const h = await headers();
  return slugFromHost(h.get("host")) ?? process.env.DEFAULT_STORE_SLUG ?? null;
}

async function sessionHeaders(): Promise<Record<string, string>> {
  const cookieStore = await cookies();
  const session = cookieStore.get("mp_customer_session")?.value;
  const out: Record<string, string> = {};
  if (session) out.Cookie = `mp_customer_session=${session}`;
  return out;
}

export async function PATCH(
  req: Request,
  ctx: { params: Promise<{ id: string }> },
): Promise<Response> {
  const slug = await resolveSlug();
  const { id } = await ctx.params;
  if (!slug) return Response.json({ error: "missing_store" }, { status: 400 });
  const body = await req.text();
  return proxyJson(
    `${marketplaceStoreUrl(slug)}/account/addresses/${encodeURIComponent(id)}`,
    { method: "PATCH", body, headers: await sessionHeaders() },
  );
}

export async function DELETE(
  _req: Request,
  ctx: { params: Promise<{ id: string }> },
): Promise<Response> {
  const slug = await resolveSlug();
  const { id } = await ctx.params;
  if (!slug) return Response.json({ error: "missing_store" }, { status: 400 });
  return proxyJson(
    `${marketplaceStoreUrl(slug)}/account/addresses/${encodeURIComponent(id)}`,
    { method: "DELETE", headers: await sessionHeaders() },
  );
}
