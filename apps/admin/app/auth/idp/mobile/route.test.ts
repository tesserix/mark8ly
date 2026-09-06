import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const configMock = vi.hoisted(() => ({
  authProvider: "zitadel" as "gip" | "zitadel",
  zitadelIssuer: "https://auth.tesserix.app",
}));
vi.mock("@/lib/config", () => ({ publicConfig: configMock }));

import { GET } from "./route";

const CANONICAL_HOST = "admin.mark8ly.com";

function makeRequest(search: string): Request {
  return new Request(`https://${CANONICAL_HOST}/auth/idp/mobile${search}`, {
    headers: { host: CANONICAL_HOST, "x-forwarded-proto": "https" },
  });
}

function location(res: Response): URL {
  const loc = res.headers.get("location");
  expect(loc).toBeTruthy();
  return new URL(loc as string);
}

beforeEach(() => {
  configMock.authProvider = "zitadel";
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("GET /auth/idp/mobile", () => {
  it("404s under GIP — nothing can reach this route outside the Zitadel provider", async () => {
    configMock.authProvider = "gip";

    const res = await GET(makeRequest("?id=i1&token=t1"));

    expect(res.status).toBe(404);
  });

  // The whole reason this page exists: the authentication session in the
  // app only closes when the browser is sent to the custom scheme, and
  // auth-bff's allowlist can only ever return to an https host.
  it("redirects a success callback to the app's own scheme, query intact", async () => {
    const res = await GET(makeRequest("?id=i1&token=t1"));

    expect(res.status).toBe(302);
    const url = location(res);
    expect(url.protocol).toBe("mark8ly-admin:");
    expect(`${url.host}${url.pathname}`).toBe("auth/idp");
    expect(url.searchParams.get("id")).toBe("i1");
    expect(url.searchParams.get("token")).toBe("t1");
  });

  // A cancelled or failed Google sign-in must come back to the app too.
  // Dropping the error params leaves the browser sitting on this page and
  // the app waiting forever — which reads as a frozen sign-in, not a
  // cancelled one.
  it("forwards a failure callback rather than dead-ending on this page", async () => {
    const res = await GET(
      makeRequest("?id=i1&error=access_denied&error_description=user%20cancelled"),
    );

    expect(res.status).toBe(302);
    const url = location(res);
    expect(url.protocol).toBe("mark8ly-admin:");
    expect(url.searchParams.get("error")).toBe("access_denied");
    expect(url.searchParams.get("error_description")).toBe("user cancelled");
    expect(url.searchParams.get("token")).toBeNull();
  });

  // This route's redirect target must not be influenceable by the URL it
  // was reached with — otherwise it becomes a way to smuggle arbitrary
  // values into the app's deep-link handler.
  it("forwards only the four Zitadel params and drops everything else", async () => {
    const res = await GET(
      makeRequest("?id=i1&token=t1&next=https%3A%2F%2Fevil.example.com&user=someone"),
    );

    const url = location(res);
    expect(url.searchParams.get("next")).toBeNull();
    // `user` is attacker-controlled and is never an identity — the app
    // must not even receive it.
    expect(url.searchParams.get("user")).toBeNull();
    expect([...url.searchParams.keys()].sort()).toEqual(["id", "token"]);
  });

  // A session left open on a blank page is the one outcome the user
  // cannot recover from, so even a malformed callback closes the session.
  it("still redirects, with an explicit error, when nothing recognisable is present", async () => {
    const res = await GET(makeRequest(""));

    expect(res.status).toBe(302);
    const url = location(res);
    expect(url.protocol).toBe("mark8ly-admin:");
    expect(url.searchParams.get("error")).toBe("invalid_callback");
  });
});
