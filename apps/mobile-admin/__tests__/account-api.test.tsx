import { createApiClient } from "@repo/mobile-shared/api/client";
import { createAccountApi } from "@repo/mobile-shared/api/account";

type Call = { method: string; path: string };

// Mirrors settings-api.test.tsx's fakeClient pattern, extended with
// deleteTenant (the tenant-scoped sibling of delete) — createAccountApi
// must route through it, not the store-scoped `delete`.
function fakeClient() {
  const calls: Call[] = [];
  const client = {
    deleteTenant: (path: string) => {
      calls.push({ method: "DELETE", path });
      return Promise.resolve(undefined);
    },
    delete: (path: string) => {
      calls.push({ method: "STORE-SCOPED-DELETE", path });
      return Promise.resolve(undefined);
    },
  } as unknown as Parameters<typeof createAccountApi>[0];
  return { client, calls };
}

describe("createAccountApi routes", () => {
  it("deleteAccount calls deleteTenant('/account'), not the store-scoped delete", async () => {
    const { client, calls } = fakeClient();
    await createAccountApi(client).deleteAccount();
    expect(calls).toEqual([{ method: "DELETE", path: "/account" }]);
  });
});

/**
 * Real routing test — mirrors api-client-204.test.tsx's pattern of mocking
 * `fetch` and asserting on the URL the client actually builds. `getTenant`
 * mounts under `/api/v1/mobile/admin` directly, skipping the active-store
 * prefix (client.ts:181-182); `deleteTenant` must do the same, even with an
 * active store present, or account deletion would 404 hitting
 * `/stores/{id}/account` instead of `/account`.
 */
describe("api client — deleteTenant routing", () => {
  const realFetch = globalThis.fetch;
  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  it("skips the /stores/{id} prefix even when a store is active", async () => {
    const fetchMock = jest.fn().mockResolvedValue({
      status: 204,
      ok: true,
      json: () => Promise.reject(new SyntaxError("Unexpected end of JSON input")),
      text: async () => "",
    } as unknown as Response);
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    const client = createApiClient({
      baseUrl: "https://x.test",
      getToken: async () => "t",
      getStoreId: () => "active-store-id",
    });

    await client.deleteTenant("/account");

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("https://x.test/api/v1/mobile/admin/account");
    expect(url).not.toContain("/stores/");
    expect(init.method).toBe("DELETE");
  });
});
