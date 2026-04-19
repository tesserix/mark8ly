import { NextRequest, NextResponse } from "next/server";
import { describe, expect, it } from "vitest";
import { applyGeoCookie } from "../../../lib/geo/geoMiddleware";

function makeRequest(
  pathname: string,
  countryCode?: string,
): NextRequest {
  const url = `http://localhost${pathname}`;
  const init: RequestInit & { headers?: Record<string, string> } = {};
  if (countryCode !== undefined) {
    init.headers = { "CF-IPCountry": countryCode };
  }
  return new NextRequest(url, init);
}

describe("applyGeoCookie", () => {
  it("sets CAD cookie when CF-IPCountry=CA", () => {
    const req = makeRequest("/pricing", "CA");
    const res = NextResponse.next();
    applyGeoCookie(req, res);

    const cookie = res.cookies.get("mk8_currency");
    expect(cookie).toBeDefined();
    expect(cookie?.value).toBe("CAD");
  });

  it("sets INR cookie when CF-IPCountry=IN", () => {
    const req = makeRequest("/pricing", "IN");
    const res = NextResponse.next();
    applyGeoCookie(req, res);

    const cookie = res.cookies.get("mk8_currency");
    expect(cookie?.value).toBe("INR");
  });

  it("sets GBP cookie when CF-IPCountry=GB", () => {
    const req = makeRequest("/pricing", "GB");
    const res = NextResponse.next();
    applyGeoCookie(req, res);

    expect(res.cookies.get("mk8_currency")?.value).toBe("GBP");
  });

  it("sets EUR cookie for EU member state (DE)", () => {
    const req = makeRequest("/pricing", "DE");
    const res = NextResponse.next();
    applyGeoCookie(req, res);

    expect(res.cookies.get("mk8_currency")?.value).toBe("EUR");
  });

  it("defaults to USD when CF-IPCountry header is missing", () => {
    const req = makeRequest("/pricing");
    const res = NextResponse.next();
    applyGeoCookie(req, res);

    const cookie = res.cookies.get("mk8_currency");
    expect(cookie?.value).toBe("USD");
  });

  it("defaults to USD for an unsupported country code", () => {
    const req = makeRequest("/pricing", "RU");
    const res = NextResponse.next();
    applyGeoCookie(req, res);

    expect(res.cookies.get("mk8_currency")?.value).toBe("USD");
  });

  it("cookie has the correct path", () => {
    const req = makeRequest("/pricing", "SG");
    const res = NextResponse.next();
    applyGeoCookie(req, res);

    const setCookieHeader = res.headers.get("set-cookie") ?? "";
    expect(setCookieHeader).toContain("Path=/");
  });

  it("cookie has the correct maxAge of 86400", () => {
    const req = makeRequest("/pricing", "AU");
    const res = NextResponse.next();
    applyGeoCookie(req, res);

    const setCookieHeader = res.headers.get("set-cookie") ?? "";
    expect(setCookieHeader).toContain("Max-Age=86400");
  });

  it("cookie name is mk8_currency", () => {
    const req = makeRequest("/pricing", "US");
    const res = NextResponse.next();
    applyGeoCookie(req, res);

    const cookie = res.cookies.get("mk8_currency");
    expect(cookie).toBeDefined();
  });

  it("does not mutate the request object", () => {
    const req = makeRequest("/pricing", "CA");
    const requestHeadersBefore = req.headers.get("CF-IPCountry");
    const res = NextResponse.next();

    applyGeoCookie(req, res);

    expect(req.headers.get("CF-IPCountry")).toBe(requestHeadersBefore);
  });
});
