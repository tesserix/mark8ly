import { createApiClient } from "@repo/mobile-shared/api/client";

function jsonResponse(status: number): Response {
  return { status, ok: status < 400, json: async () => ({}), text: async () => "" } as Response;
}

describe("onUnauthorized reason", () => {
  const realFetch = global.fetch;
  afterEach(() => {
    global.fetch = realFetch;
  });

  it("reports no-session when there is no token at all", async () => {
    const onUnauthorized = jest.fn();
    global.fetch = jest.fn() as unknown as typeof fetch;
    const client = createApiClient({
      baseUrl: "https://x.test",
      getToken: async () => null,
      getStoreId: () => null,
      onUnauthorized,
    });
    await expect(client.get("/stores")).rejects.toBeTruthy();
    expect(onUnauthorized).toHaveBeenCalledWith("no-session");
  });

  it("reports no-session when the refresh cannot mint a token", async () => {
    const onUnauthorized = jest.fn();
    global.fetch = jest.fn().mockResolvedValue(jsonResponse(401)) as unknown as typeof fetch;
    const client = createApiClient({
      baseUrl: "https://x.test",
      getToken: async () => "stale",
      refreshToken: async () => null,
      getStoreId: () => null,
      onUnauthorized,
    });
    await expect(client.get("/stores")).rejects.toBeTruthy();
    expect(onUnauthorized).toHaveBeenCalledWith("no-session");
  });

  it("reports access-denied when a FRESH token is still rejected", async () => {
    // A freshly minted token that the server still 401s is not expiry —
    // it is the server refusing this identity.
    const onUnauthorized = jest.fn();
    global.fetch = jest.fn().mockResolvedValue(jsonResponse(401)) as unknown as typeof fetch;
    const client = createApiClient({
      baseUrl: "https://x.test",
      getToken: async () => "stale",
      refreshToken: async () => "fresh",
      getStoreId: () => null,
      onUnauthorized,
    });
    await expect(client.get("/stores")).rejects.toBeTruthy();
    expect(onUnauthorized).toHaveBeenCalledWith("access-denied");
  });

  it("does not call onUnauthorized when the retry succeeds", async () => {
    const onUnauthorized = jest.fn();
    global.fetch = jest
      .fn()
      .mockResolvedValueOnce(jsonResponse(401))
      .mockResolvedValueOnce(jsonResponse(200)) as unknown as typeof fetch;
    const client = createApiClient({
      baseUrl: "https://x.test",
      getToken: async () => "stale",
      refreshToken: async () => "fresh",
      getStoreId: () => null,
      onUnauthorized,
    });
    await client.get("/stores").catch(() => undefined);
    expect(onUnauthorized).not.toHaveBeenCalled();
  });
});
