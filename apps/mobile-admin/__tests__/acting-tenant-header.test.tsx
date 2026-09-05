import { createApiClient } from "@repo/mobile-shared/api/client";

// marketplace-api resolves tenancy for a Zitadel token from the
// X-Acting-Tenant-Id header, FGA-checked against the caller (#686). A GIP
// token carried a tenant claim and needed none of this; a Zitadel token
// carries no claim at all, so without the header every request resolves no
// tenant and is refused 404.

function jsonResponse(body: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    headers: { get: () => "application/json" },
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(JSON.stringify(body)),
  } as unknown as Response);
}

function clientWith(actingTenantId: string | null, storeId: string | null = null) {
  const fetchMock = jest.fn(() => jsonResponse({ data: [] }));
  global.fetch = fetchMock as unknown as typeof fetch;
  const client = createApiClient({
    baseUrl: "https://api.mark8ly.com",
    getToken: async () => "tok",
    getStoreId: () => storeId,
    getActingTenantId: () => actingTenantId,
  });
  return { client, fetchMock };
}

function headersOf(fetchMock: jest.Mock): Record<string, string> {
  return (fetchMock.mock.calls[0][1] as RequestInit).headers as Record<string, string>;
}

it("sends X-Acting-Tenant-Id when a tenant is known", async () => {
  const { client, fetchMock } = clientWith("e638b731-6a49-48ce-8fad-80cc1e6213c2");
  await client.getTenant("/stores");

  expect(headersOf(fetchMock)["X-Acting-Tenant-Id"]).toBe(
    "e638b731-6a49-48ce-8fad-80cc1e6213c2",
  );
});

it("omits the header entirely when no tenant is known", async () => {
  const { client, fetchMock } = clientWith(null);
  await client.getTenant("/stores");

  // An empty-string header is NOT the same as an absent one: the server
  // treats a present-but-empty value as a stated tenant and fails the FGA
  // check, turning "not resolved yet" into a hard refusal.
  expect(headersOf(fetchMock)).not.toHaveProperty("X-Acting-Tenant-Id");
});

// The regression this guards: tenant-store used to write the STORE id into
// the tenant slot. Sending a store id here would fail every FGA membership
// check, and the failure surfaces as 404 "no store" — indistinguishable
// from a genuinely unbound account.
it("sends the tenant id, never the store id", async () => {
  const { client, fetchMock } = clientWith("tenant-uuid", "store-uuid");
  await client.get("/products");

  const headers = headersOf(fetchMock);
  expect(headers["X-Acting-Tenant-Id"]).toBe("tenant-uuid");
  expect(headers["X-Acting-Tenant-Id"]).not.toBe("store-uuid");
});

// Configuration is optional so a GIP build, whose token still carries the
// claim, is byte-for-byte unchanged.
it("omits the header when no resolver is configured at all", async () => {
  const fetchMock = jest.fn(() => jsonResponse({ data: [] }));
  global.fetch = fetchMock as unknown as typeof fetch;
  const client = createApiClient({
    baseUrl: "https://api.mark8ly.com",
    getToken: async () => "tok",
    getStoreId: () => null,
  });

  await client.get("/products");
  expect(headersOf(fetchMock)).not.toHaveProperty("X-Acting-Tenant-Id");
});
