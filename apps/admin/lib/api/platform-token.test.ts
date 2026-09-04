import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { __resetPlatformTokenCache, getPlatformToken } from "./platform-token";

// The machine token mark8ly presents to tesserix-home's platform API.
//
// Worth testing rather than trusting because two of its failure modes are
// silent: a token minted WITHOUT the roles scope verifies against the JWKS and
// still answers 401 (it carries no roles claim), and a cache that ignores
// expiry works perfectly until the hour it does not.

const ENV = {
  PLATFORM_OIDC_ISSUER: "https://auth.tesserix.app",
  PLATFORM_OIDC_PROJECT_ID: "386377618200461939",
  PLATFORM_OIDC_CLIENT_ID: "mark8ly-catalog-reader",
  PLATFORM_OIDC_CLIENT_SECRET: "s3cret",
};

function tokenResponse(expiresIn = 3600, token = "tok-1") {
  return {
    ok: true,
    status: 200,
    json: async () => ({ access_token: token, expires_in: expiresIn }),
    text: async () => "",
  } as unknown as Response;
}

beforeEach(() => {
  __resetPlatformTokenCache();
  Object.assign(process.env, ENV);
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  for (const k of Object.keys(ENV)) delete process.env[k];
});

describe("minting", () => {
  it("asks for the roles scope, without which the token 401s at the API", async () => {
    const fetchMock = vi.fn().mockResolvedValue(tokenResponse());
    vi.stubGlobal("fetch", fetchMock);

    await getPlatformToken();

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("https://auth.tesserix.app/oauth/v2/token");
    const body = String((init as RequestInit).body);
    expect(body).toContain("grant_type=client_credentials");
    // The two scopes that matter. Measured against production during #152:
    // the audience scope alone yields a token with NO roles claim, which the
    // API refuses with 401 — indistinguishable from a bad credential.
    expect(decodeURIComponent(body)).toContain(
      "urn:zitadel:iam:org:project:id:386377618200461939:aud",
    );
    expect(decodeURIComponent(body)).toContain("urn:zitadel:iam:org:projects:roles");
  });

  it("authenticates the client rather than putting the secret in the body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(tokenResponse());
    vi.stubGlobal("fetch", fetchMock);

    await getPlatformToken();

    const init = fetchMock.mock.calls[0][1] as RequestInit;
    const auth = new Headers(init.headers).get("authorization") ?? "";
    expect(auth.startsWith("Basic ")).toBe(true);
    expect(String(init.body)).not.toContain("s3cret");
  });
});

describe("caching", () => {
  it("reuses a live token instead of minting per request", async () => {
    const fetchMock = vi.fn().mockResolvedValue(tokenResponse());
    vi.stubGlobal("fetch", fetchMock);

    const a = await getPlatformToken();
    const b = await getPlatformToken();

    expect(a).toBe(b);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("re-mints BEFORE expiry rather than at it", async () => {
    // A token that is valid when checked can still be expired when it
    // arrives. The skew is what stops a support page failing for one
    // merchant every hour.
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(tokenResponse(3600, "tok-1"))
      .mockResolvedValueOnce(tokenResponse(3600, "tok-2"));
    vi.stubGlobal("fetch", fetchMock);

    expect(await getPlatformToken()).toBe("tok-1");
    // Just inside the refresh skew — still short of the real expiry.
    vi.advanceTimersByTime((3600 - 30) * 1000);
    expect(await getPlatformToken()).toBe("tok-2");
  });

  it("does not stampede when several requests arrive together", async () => {
    // The support page fetches list and announcements at once; a cold cache
    // must mint once, not once per caller.
    const fetchMock = vi.fn().mockResolvedValue(tokenResponse());
    vi.stubGlobal("fetch", fetchMock);

    await Promise.all([getPlatformToken(), getPlatformToken(), getPlatformToken()]);

    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("does not cache a failure", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: false, status: 503, text: async () => "upstream" } as unknown as Response)
      .mockResolvedValueOnce(tokenResponse());
    vi.stubGlobal("fetch", fetchMock);

    await expect(getPlatformToken()).rejects.toThrow();
    // A transient mint failure must not poison the cache for the next caller.
    await expect(getPlatformToken()).resolves.toBe("tok-1");
  });
});

describe("configuration", () => {
  it("refuses with a message naming what is missing", async () => {
    delete process.env.PLATFORM_OIDC_CLIENT_SECRET;
    vi.stubGlobal("fetch", vi.fn());

    await expect(getPlatformToken()).rejects.toThrow(/PLATFORM_OIDC_CLIENT_SECRET/);
  });
});
