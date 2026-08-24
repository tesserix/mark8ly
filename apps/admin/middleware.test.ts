import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { NextRequest } from "next/server";

import { middleware } from "./middleware";

const SLUG = "acme";
const TENANT_ID = "tenant-123";
const USER_ID = "user-456";
const SESSION_COOKIE_NAME = process.env.SESSION_COOKIE_NAME ?? "m8_session";

// Builds a request against `{slug}-admin.mark8ly.com/dashboard` carrying a
// valid session cookie value — the value itself is never inspected because
// auth-bff's response (mocked below) is the only thing that matters.
function buildRequest(): NextRequest {
  const req = new NextRequest(
    `https://${SLUG}-admin.mark8ly.com/dashboard`,
    { headers: { host: `${SLUG}-admin.mark8ly.com`, cookie: `${SESSION_COOKIE_NAME}=valid-session-token` } },
  );
  return req;
}

// Routes the mocked global fetch by URL substring so the test doesn't care
// about call order — the middleware makes several sequential fetches
// (slug-status, session, store-by-slug, role) before it finishes.
function mockFetchForStatus(status: string) {
  global.fetch = vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === "string" ? input : input.toString();

    if (url.includes("/internal/store-active-domain/")) {
      return new Response(JSON.stringify({ custom_domain: "" }), { status: 200 });
    }
    if (url.includes("/auth/session")) {
      return new Response(
        JSON.stringify({
          data: {
            user_id: USER_ID,
            email: "merchant@example.com",
            tenant_id: TENANT_ID,
          },
        }),
        { status: 200 },
      );
    }
    if (url.includes("/internal/stores/by-slug/")) {
      return new Response(
        JSON.stringify({ data: { tenant_id: TENANT_ID, status } }),
        { status: 200 },
      );
    }
    if (url.includes("/internal/tenants/") && url.includes("/me")) {
      return new Response(JSON.stringify({ data: { role: "owner" } }), {
        status: 200,
      });
    }
    throw new Error(`unexpected fetch: ${url}`);
  }) as unknown as typeof fetch;
}

// Mocks the by-slug 404 branch specifically (line ~339 in middleware.ts) —
// the store-active-domain lookup and session both succeed, but
// platform-api's /internal/stores/by-slug/:slug returns 404, which is the
// pre-existing "unknown slug" branch the new status check sits right below.
function mockFetchBySlugNotFound() {
  global.fetch = vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === "string" ? input : input.toString();
    if (url.includes("/internal/store-active-domain/")) {
      return new Response(JSON.stringify({ custom_domain: "" }), { status: 200 });
    }
    if (url.includes("/auth/session")) {
      return new Response(
        JSON.stringify({
          data: { user_id: USER_ID, email: "merchant@example.com", tenant_id: TENANT_ID },
        }),
        { status: 200 },
      );
    }
    if (url.includes("/internal/stores/by-slug/")) {
      return new Response(null, { status: 404 });
    }
    throw new Error(`unexpected fetch: ${url}`);
  }) as unknown as typeof fetch;
}

describe("admin middleware — tenant status gate", () => {
  const originalFetch = global.fetch;

  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    global.fetch = originalFetch;
  });

  it("404s a suspended store and never reaches the tenant-switch path", async () => {
    mockFetchForStatus("suspended");
    const res = await middleware(buildRequest());
    expect(res.status).toBe(404);

    // The tenant-switch path calls auth-bff's /auth/switch-tenant — assert
    // it was never hit, proving the refusal short-circuits before it.
    const calledUrls = (global.fetch as ReturnType<typeof vi.fn>).mock.calls.map(
      (call) => String(call[0]),
    );
    expect(calledUrls.some((u) => u.includes("/auth/switch-tenant"))).toBe(false);
  });

  it("404s an archived store", async () => {
    mockFetchForStatus("archived");
    const res = await middleware(buildRequest());
    expect(res.status).toBe(404);
  });

  it("does not 404 an active store (regression guard)", async () => {
    mockFetchForStatus("active");
    const res = await middleware(buildRequest());
    expect(res.status).not.toBe(404);
  });

  it("still 404s when platform-api reports the slug unknown (pre-existing behaviour)", async () => {
    mockFetchBySlugNotFound();
    const res = await middleware(buildRequest());
    expect(res.status).toBe(404);
  });
});
