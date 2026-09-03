import { NextRequest } from "next/server";
import { describe, expect, it } from "vitest";

import { GET } from "./route";
import {
  ZITADEL_STATE_COOKIE,
  ZITADEL_RETURN_URL_COOKIE,
  ZITADEL_VERIFIER_COOKIE,
} from "@/lib/auth/zitadel-oidc";

function makeRequest(
  search: string,
  cookies: Record<string, string> = {},
): NextRequest {
  const url = `https://admin.mark8ly.com/auth/callback${search}`;
  const cookieHeader = Object.entries(cookies)
    .map(([name, value]) => `${name}=${encodeURIComponent(value)}`)
    .join("; ");
  return new NextRequest(url, {
    headers: cookieHeader ? { cookie: cookieHeader } : undefined,
  });
}

describe("GET /auth/callback", () => {
  it("redirects (303) to the sanitised destination when state matches", async () => {
    const req = makeRequest("?code=abc&state=s1", {
      [ZITADEL_STATE_COOKIE]: "s1",
      [ZITADEL_RETURN_URL_COOKIE]: "https://the-bondi-store-admin.mark8ly.com/dashboard",
    });

    const res = await GET(req);

    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toBe(
      "https://the-bondi-store-admin.mark8ly.com/dashboard",
    );
  });

  it("falls back to /dashboard when no return-url cookie was set", async () => {
    const req = makeRequest("?code=abc&state=s1", {
      [ZITADEL_STATE_COOKIE]: "s1",
    });

    const res = await GET(req);

    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toBe("https://admin.mark8ly.com/dashboard");
  });

  it("rejects a callback whose state does not match the cookie", async () => {
    const req = makeRequest("?code=abc&state=attacker-supplied", {
      [ZITADEL_STATE_COOKIE]: "s1",
      [ZITADEL_RETURN_URL_COOKIE]: "https://the-bondi-store-admin.mark8ly.com/dashboard",
    });

    const res = await GET(req);

    expect(res.status).toBe(303);
    const location = res.headers.get("location") ?? "";
    expect(location).toContain("/login");
    expect(location).toContain("error=state_mismatch");
    // Never follows the attacker's implied destination.
    expect(location).not.toContain("the-bondi-store-admin");
  });

  it("rejects a callback with no state cookie at all", async () => {
    const req = makeRequest("?code=abc&state=s1");

    const res = await GET(req);

    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toContain("error=state_mismatch");
  });

  it("rejects a callback with no state query param", async () => {
    const req = makeRequest("?code=abc", {
      [ZITADEL_STATE_COOKIE]: "s1",
    });

    const res = await GET(req);

    expect(res.status).toBe(303);
    expect(res.headers.get("location")).toContain("error=state_mismatch");
  });

  it("never follows an off-domain destination even on a matching state", async () => {
    const req = makeRequest("?code=abc&state=s1", {
      [ZITADEL_STATE_COOKIE]: "s1",
      [ZITADEL_RETURN_URL_COOKIE]: "https://evil.example.com/steal",
    });

    const res = await GET(req);

    expect(res.status).toBe(303);
    // Sanitizer rejects the off-domain cookie value, so we fall back
    // to the safe default instead of redirecting to it.
    expect(res.headers.get("location")).toBe("https://admin.mark8ly.com/dashboard");
  });

  it("clears the state, verifier and return-url cookies on a successful callback", async () => {
    const req = makeRequest("?code=abc&state=s1", {
      [ZITADEL_STATE_COOKIE]: "s1",
      [ZITADEL_VERIFIER_COOKIE]: "verifier-value",
      [ZITADEL_RETURN_URL_COOKIE]: "https://the-bondi-store-admin.mark8ly.com/dashboard",
    });

    const res = await GET(req);

    for (const name of [
      ZITADEL_STATE_COOKIE,
      ZITADEL_VERIFIER_COOKIE,
      ZITADEL_RETURN_URL_COOKIE,
    ]) {
      const cookie = res.cookies.get(name);
      expect(cookie?.value).toBe("");
      expect(cookie?.maxAge).toBe(0);
    }
  });

  it("also clears the state cookie when a callback is rejected (single-use even on failure)", async () => {
    const req = makeRequest("?code=abc&state=wrong", {
      [ZITADEL_STATE_COOKIE]: "s1",
    });

    const res = await GET(req);

    expect(res.cookies.get(ZITADEL_STATE_COOKIE)?.value).toBe("");
  });
});
