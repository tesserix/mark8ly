import { describe, it, expect } from "vitest";
import { isCrossOriginStateChange } from "./csrf";

function req(
  method: string,
  host: string,
  headers: Record<string, string> = {},
) {
  return {
    method,
    headers: new Headers({ host, ...headers }),
  };
}

describe("isCrossOriginStateChange", () => {
  it("allows safe methods regardless of origin", () => {
    for (const method of ["GET", "HEAD", "OPTIONS"]) {
      const r = req(method, "admin.mark8ly.com", {
        origin: "https://evil.com",
        "sec-fetch-site": "cross-site",
      });
      expect(isCrossOriginStateChange(r)).toBe(false);
    }
  });

  it("blocks a cross-site POST", () => {
    const r = req("POST", "admin.mark8ly.com", {
      origin: "https://evil.com",
      "sec-fetch-site": "cross-site",
    });
    expect(isCrossOriginStateChange(r)).toBe(true);
  });

  it("blocks a sibling subdomain, which SameSite=Lax lets through", () => {
    const r = req("POST", "victim-admin.mark8ly.com", {
      origin: "https://attacker-admin.mark8ly.com",
      "sec-fetch-site": "same-site",
    });
    expect(isCrossOriginStateChange(r)).toBe(true);
  });

  it("allows a same-origin POST", () => {
    const r = req("POST", "admin.mark8ly.com", {
      origin: "https://admin.mark8ly.com",
      "sec-fetch-site": "same-origin",
    });
    expect(isCrossOriginStateChange(r)).toBe(false);
  });

  it("compares host and port together in dev", () => {
    const r = req("POST", "localhost:3001", {
      origin: "http://localhost:3001",
    });
    expect(isCrossOriginStateChange(r)).toBe(false);
  });

  it("uses x-forwarded-host when present, as behind the gateway", () => {
    const r = req("PATCH", "10.0.0.5:3001", {
      "x-forwarded-host": "admin.mark8ly.com",
      origin: "https://admin.mark8ly.com",
    });
    expect(isCrossOriginStateChange(r)).toBe(false);
  });

  it("allows requests with no Origin and no Sec-Fetch-Site", () => {
    // Non-browser callers (mobile apps, server-to-server) send neither.
    // A browser always sends Origin on a cross-origin state change, so
    // absence is not evidence of an attack.
    expect(isCrossOriginStateChange(req("POST", "admin.mark8ly.com"))).toBe(
      false,
    );
  });

  it("blocks an opaque origin", () => {
    const r = req("POST", "admin.mark8ly.com", { origin: "null" });
    expect(isCrossOriginStateChange(r)).toBe(true);
  });

  it("blocks a malformed origin", () => {
    const r = req("POST", "admin.mark8ly.com", { origin: "not-a-url" });
    expect(isCrossOriginStateChange(r)).toBe(true);
  });

  it("trusts Sec-Fetch-Site: same-origin when Origin is absent", () => {
    const r = req("DELETE", "admin.mark8ly.com", {
      "sec-fetch-site": "same-origin",
    });
    expect(isCrossOriginStateChange(r)).toBe(false);
  });
});
