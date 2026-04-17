// Server-side proxy for the Otto staff console. Reads the session headers
// set by the admin middleware, forwards them to the otto service, and pipes
// the response back. Session validation has already happened upstream — we
// simply trust and forward.

import { headers } from "next/headers";
import { NextResponse } from "next/server";

const OTTO_URL = process.env.OTTO_URL ?? "http://localhost:8089";
const OTTO_INTERNAL_AUTH = process.env.OTTO_INTERNAL_AUTH ?? "";

interface ForwardInit {
  method?: string;
  body?: unknown;
}

export async function forwardToOtto(
  path: string,
  init: ForwardInit = {},
): Promise<Response> {
  const h = await headers();
  const userId = h.get("x-session-user-id") ?? "";
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const storeId = h.get("x-session-store-id") ?? "";
  const email = h.get("x-session-email") ?? "";

  if (!userId || !tenantId) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }
  if (!storeId) {
    return NextResponse.json(
      { error: "no_active_store", message: "pick a store first" },
      { status: 400 },
    );
  }

  const outgoing: Record<string, string> = {
    "Content-Type": "application/json",
    "X-User-Id": userId,
    "X-Tenant-Id": tenantId,
    "X-Store-Id": storeId,
  };
  if (email) outgoing["X-User-Email"] = email;
  if (OTTO_INTERNAL_AUTH) outgoing["X-Internal-Auth"] = OTTO_INTERNAL_AUTH;

  try {
    const upstream = await fetch(`${OTTO_URL}${path}`, {
      method: init.method ?? "GET",
      headers: outgoing,
      body: init.body === undefined ? undefined : JSON.stringify(init.body),
      cache: "no-store",
    });
    const text = await upstream.text();
    return new Response(text, {
      status: upstream.status,
      headers: {
        "Content-Type":
          upstream.headers.get("Content-Type") ?? "application/json",
      },
    });
  } catch (err) {
    return NextResponse.json(
      {
        error: "upstream_unreachable",
        message:
          err instanceof Error ? err.message : "otto service unreachable",
      },
      { status: 502 },
    );
  }
}
