import { createApiClient } from "@repo/mobile-shared/api/client";

/**
 * A genuine 204 No Content response has an EMPTY body — calling `res.json()`
 * on it throws `SyntaxError: Unexpected end of JSON input` in React Native.
 * `updateMedia` (media.go:195) and `deleteMedia` (media.go:261) both return
 * 204, so without a short-circuit every successful reorder / alt-text / delete
 * would surface as a mutation error and `invalidateQueries` would never run.
 *
 * This mocks `json()` to reject exactly as RN does on an empty body, and
 * asserts the client resolves to `undefined` instead of propagating that
 * rejection. Sibling clients already guard this (storefront-client.ts:104,
 * support/client.ts:127) — the admin client must too.
 */
function noContentResponse(): Response {
  return {
    status: 204,
    ok: true,
    json: () => Promise.reject(new SyntaxError("Unexpected end of JSON input")),
    text: async () => "",
  } as unknown as Response;
}

describe("api client — 204 No Content", () => {
  const realFetch = globalThis.fetch;
  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  function client() {
    globalThis.fetch = jest.fn().mockResolvedValue(noContentResponse()) as unknown as typeof fetch;
    return createApiClient({
      baseUrl: "https://x.test",
      getToken: async () => "t",
      getStoreId: () => null,
    });
  }

  it("resolves a PATCH that returns 204 to undefined (updateMedia)", async () => {
    await expect(
      client().patch("/products/p1/media/m1", { position: 0 }),
    ).resolves.toBeUndefined();
  });

  it("resolves a DELETE that returns 204 to undefined (deleteMedia)", async () => {
    await expect(client().delete("/products/p1/media/m1")).resolves.toBeUndefined();
  });
});
