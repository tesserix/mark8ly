import { NextRequest } from "next/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// Important 2 (whole-branch review, phase 3a): before this fix, this
// route was reachable regardless of the provider flag — it set three
// flow cookies on any anonymous GET and 500'd when the issuer was
// unset. It should not exist under GIP at all.
const configMock = vi.hoisted(() => ({ authProvider: "zitadel" as "gip" | "zitadel" }));
vi.mock("@/lib/config", () => ({ publicConfig: configMock }));

import { GET } from "./route";

function makeRequest(search = ""): NextRequest {
  return new NextRequest(`https://admin.mark8ly.com/login/authorize${search}`, {
    headers: { host: "admin.mark8ly.com" },
  });
}

beforeEach(() => {
  configMock.authProvider = "zitadel";
  vi.stubEnv("NEXT_PUBLIC_ZITADEL_ISSUER", "https://auth.tesserix.app");
  vi.stubEnv("NEXT_PUBLIC_ZITADEL_ADMIN_CLIENT_ID", "admin-client-id");
});

afterEach(() => {
  vi.unstubAllEnvs();
  configMock.authProvider = "zitadel";
});

describe("GET /login/authorize — provider gate", () => {
  it("404s when the provider is not zitadel, never minting flow cookies", async () => {
    configMock.authProvider = "gip";

    const res = await GET(makeRequest());

    expect(res.status).toBe(404);
    expect((res as Response).headers.get("set-cookie")).toBeNull();
  });

  it("redirects to Zitadel's /authorize when the provider is zitadel and the issuer is configured", async () => {
    const res = await GET(makeRequest());

    expect(res.status).toBe(307);
    const location = res.headers.get("location") ?? "";
    expect(location).toContain("https://auth.tesserix.app/oauth/v2/authorize");
  });
});

describe("GET /login/authorize — redirect_uri is pinned to the canonical origin", () => {
  // Production incident (#524): the post-accept handoff in accept-invite
  // jumps straight to this route on the host the invitation link opened —
  // a {slug}-admin host — skipping the canonical bounce /login performs.
  // redirect_uri was built from that host, and Zitadel refused it with
  // "The requested redirect_uri is missing in the client configuration",
  // because the admin OIDC app registers exactly one callback.
  function slugHostRequest(): NextRequest {
    return new NextRequest(
      "https://the-bondi-store-admin.mark8ly.com/login/authorize?returnUrl=%2Fdashboard",
      {
        headers: {
          host: "the-bondi-store-admin.mark8ly.com",
          "x-forwarded-host": "the-bondi-store-admin.mark8ly.com",
          "x-forwarded-proto": "https",
        },
      },
    );
  }

  it("uses the canonical origin even when the request arrives on a slug host", async () => {
    vi.stubEnv("NEXT_PUBLIC_ADMIN_LOGIN_ORIGIN", "https://admin.mark8ly.com");
    const res = await GET(slugHostRequest());
    const redirectUri = new URL(
      res.headers.get("location") ?? "",
    ).searchParams.get("redirect_uri");
    expect(redirectUri).toBe("https://admin.mark8ly.com/auth/callback");
    expect(redirectUri).not.toContain("the-bondi-store-admin");
  });

  it("falls back to the request origin when no canonical origin is configured", async () => {
    // Dev/preview: same-origin is the only sane default, and matches how
    // middleware.ts treats an unset NEXT_PUBLIC_ADMIN_LOGIN_ORIGIN.
    vi.stubEnv("NEXT_PUBLIC_ADMIN_LOGIN_ORIGIN", "");
    const res = await GET(slugHostRequest());
    const redirectUri = new URL(
      res.headers.get("location") ?? "",
    ).searchParams.get("redirect_uri");
    expect(redirectUri).toBe(
      "https://the-bondi-store-admin.mark8ly.com/auth/callback",
    );
  });
});

// The other half of the spent-auth-request fix: /login sends a `recovery`
// arrival here to mint a FRESH auth request, and the truthful Google
// failure message has to survive the hop. Zitadel builds the final
// /login?authRequest=… URL itself from its configured login base URI and
// appends only its own param, so a query string cannot carry the message —
// it rides a short-lived cookie, exactly like returnUrl already does.
describe("GET /login/authorize — carrying a Google outcome code across the Zitadel hop", () => {
  function setCookieHeaders(res: Response): string[] {
    const getSetCookie = (res.headers as unknown as {
      getSetCookie?: () => string[];
    }).getSetCookie;
    if (typeof getSetCookie === "function") return getSetCookie.call(res.headers);
    const raw = res.headers.get("set-cookie");
    return raw ? [raw] : [];
  }

  function errorCookie(res: Response): string | undefined {
    return setCookieHeaders(res).find((c) => c.startsWith("zt_login_error="));
  }

  it("stores a recognised outcome code so /login can show it after the round trip", async () => {
    const res = await GET(makeRequest("?error=no_admin_account"));

    const cookie = errorCookie(res);
    expect(cookie).toBeDefined();
    expect(cookie).toContain("zt_login_error=no_admin_account");
    expect(cookie).toContain("HttpOnly");
    expect(cookie).toContain("Max-Age=120");
  });

  it("still redirects to Zitadel — carrying the message must not change where the browser goes", async () => {
    const res = await GET(makeRequest("?error=no_admin_account"));

    expect(res.headers.get("location")).toContain(
      "https://auth.tesserix.app/oauth/v2/authorize",
    );
  });

  it("leaves the PKCE state and verifier cookies untouched", async () => {
    const withError = setCookieHeaders(await GET(makeRequest("?error=no_admin_account")));
    const withoutError = setCookieHeaders(await GET(makeRequest()));

    for (const cookies of [withError, withoutError]) {
      expect(cookies.some((c) => c.startsWith("zt_auth_state="))).toBe(true);
      expect(cookies.some((c) => c.startsWith("zt_pkce_verifier="))).toBe(true);
    }
  });

  it("refuses to store an unrecognised error value — no provider text can reach the page this way", async () => {
    const res = await GET(
      makeRequest(
        "?error=" + encodeURIComponent('{"error": "No valid authentication request found"}'),
      ),
    );

    const cookie = errorCookie(res);
    expect(cookie).toBeDefined();
    // Present but EXPIRED and empty: an unrecognised value clears the
    // channel rather than passing anything through it.
    expect(cookie).toContain("Max-Age=0");
    expect(cookie).not.toContain("No valid authentication request found");
  });

  it("clears any stale message when a new authorize starts with no error", async () => {
    const cookie = errorCookie(await GET(makeRequest("?returnUrl=%2Fdashboard")));

    expect(cookie).toBeDefined();
    expect(cookie).toContain("Max-Age=0");
  });

  it("never mints the error cookie under GIP, where this route does not exist", async () => {
    configMock.authProvider = "gip";

    const res = await GET(makeRequest("?error=no_admin_account"));

    expect(res.status).toBe(404);
    expect(setCookieHeaders(res)).toEqual([]);
  });
});
