// End-to-end-ish test of the finish route against auth-bff's REAL wire
// shape — unlike route.test.ts (which mocks @/app/login/actions entirely,
// exercising only the route's own branching), this test stubs fetch at the
// network boundary and lets the real finishZitadelGoogleSignIn ->
// zitadelIdpFinish -> parseLoginResponse -> mapZitadelOutcome chain run, so
// the route's `data.callbackUrl` branch is genuinely exercised rather than
// only reachable in a mock that assumes a shape auth-bff never sends.
//
// The response body below is copied verbatim from
// services/auth-bff/internal/zitadellogin/handler.go's finishComplete —
// `writeJSON(w, 200, map[string]any{"callback_url": res.CallbackURL})` —
// pinned server-side by handler_test.go's
// TestLoginCompleteCallsCompleteAndReturnsCallbackURL. No `data`, no `uid`,
// no `tenant_id`.
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

process.env.SESSION_ENCRYPT_KEY ||= "thirtytwo-bytes-for-testing-only";

const configMock = vi.hoisted(() => ({
  authProvider: "zitadel" as "gip" | "zitadel",
  zitadelIssuer: "https://auth.tesserix.app",
}));
vi.mock("@/lib/config", () => ({
  publicConfig: configMock,
  config: { authBffUrl: "http://localhost:8087", platformApiUrl: "http://localhost:8086" },
}));

vi.mock("@/lib/auth/tenant-host", () => ({
  tenantIdForHostSlug: vi.fn().mockResolvedValue("tenant-1"),
}));

const cookiesSetSpy = vi.fn();
let headerMap: Map<string, string>;
vi.mock("next/headers", () => ({
  headers: async () => ({
    get: (key: string) => headerMap.get(key.toLowerCase()) ?? null,
  }),
  cookies: async () => ({
    set: cookiesSetSpy,
    getAll: () => [],
  }),
}));

import { GET } from "./route";

function makeRequest(search: string, host = "demo-store-admin.mark8ly.com"): Request {
  return new Request(`https://${host}/auth/idp/finish${search}`, {
    headers: { host, "x-forwarded-proto": "https" },
  });
}

beforeEach(() => {
  configMock.authProvider = "zitadel";
  headerMap = new Map([
    ["host", "demo-store-admin.mark8ly.com"],
    ["user-agent", "Mozilla/5.0 test-client"],
  ]);
  cookiesSetSpy.mockClear();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

function stubAuthBffFetch(status: number, body: unknown, setCookie?: string) {
  const fn = vi.fn().mockImplementation(async () => {
    const res = new Response(JSON.stringify(body), {
      status,
      headers: { "content-type": "application/json" },
    });
    if (setCookie) res.headers.append("set-cookie", setCookie);
    return res;
  });
  vi.stubGlobal("fetch", fn);
  return fn;
}

describe("GET /auth/idp/finish — real auth-bff wire shape", () => {
  it("mints m8_session and redirects to the trusted callback_url on a genuine completed sign-in", async () => {
    stubAuthBffFetch(
      200,
      { callback_url: "https://demo-store-admin.mark8ly.com/auth/callback?code=c&state=s" },
      "m8_session=abc; Path=/; HttpOnly",
    );

    const res = await GET(makeRequest("?id=intent-1&token=tok-1&auth_request_id=ar-1"));

    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toBe(
      "https://demo-store-admin.mark8ly.com/auth/callback?code=c&state=s",
    );
    expect(cookiesSetSpy).toHaveBeenCalledWith(
      expect.objectContaining({ name: "m8_session", value: "abc" }),
    );
  });

  it("a genuine rejection (no_admin_account) redirects with the error and mints no cookie", async () => {
    stubAuthBffFetch(403, { error: "no_admin_account" });

    const res = await GET(makeRequest("?id=intent-1&token=tok-1&auth_request_id=ar-1"));

    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toContain("error=no_admin_account");
    expect(cookiesSetSpy).not.toHaveBeenCalled();
  });
});
