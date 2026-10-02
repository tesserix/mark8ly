// Platform support must state the tenant it is acting as.
//
// Reported from a device: More > Tesserix Support showed "no store linked to
// this account" for an account that plainly has one.
//
// The admin platform-support group is mounted behind `requireTenant`
// (mobile_routes.go), and with GIP gone NO bearer token carries a tenant
// claim — the FGA-validated `X-Acting-Tenant-Id` header is the only way a
// tenant reaches those routes. The main API client sends it
// (packages/mobile-shared/api/client.ts); the support client is a SEPARATE
// client and had no way to supply one at all, so every request 404'd.
//
// The empty-string case matters as much as the missing one: the server treats
// a present value as a STATED tenant and fails it against FGA, so sending ""
// turns "not resolved yet" into a hard refusal.

import { createSupportClient } from "@repo/mobile-shared/support/client";

type Captured = { url: string; headers: Record<string, string> };

function harness(getActingTenantId?: () => string | null) {
  const calls: Captured[] = [];
  const fetchMock = jest.fn(async (url: string, init: RequestInit) => {
    calls.push({
      url: String(url),
      headers: (init?.headers ?? {}) as Record<string, string>,
    });
    return {
      ok: true,
      status: 200,
      json: async () => ({ conversation: { id: "c1" }, messages: [] }),
      headers: { get: () => null },
    };
  });
  (globalThis as { fetch?: unknown }).fetch = fetchMock;

  const client = createSupportClient({
    baseUrl: "https://api.mark8ly.com",
    basePath: "/api/v1/mobile/admin/platform-support",
    getToken: async () => "TOKEN",
    getActingTenantId,
  });
  return { client, calls };
}

describe("support client", () => {
  it("sends X-Acting-Tenant-Id when the tenant is known", async () => {
    const { client, calls } = harness(() => "8c302556-b647-4824-8ce4-73f547ca456e");

    await client.resume().catch(() => undefined);

    expect(calls.length).toBeGreaterThan(0);
    // Without the header the route 404s and the screen reports no store.
    expect(calls[0].headers["X-Acting-Tenant-Id"]).toBe(
      "8c302556-b647-4824-8ce4-73f547ca456e",
    );
  });

  it("omits the header entirely when no tenant is resolved yet", async () => {
    const { client, calls } = harness(() => null);

    await client.resume().catch(() => undefined);

    expect(calls.length).toBeGreaterThan(0);
    // NOT an empty string: the server reads a present value as a stated
    // tenant and fails it against FGA rather than falling through.
    expect(calls[0].headers).not.toHaveProperty("X-Acting-Tenant-Id");
  });

  it("omits it when the caller supplies no getter at all (storefront surface)", async () => {
    const { client, calls } = harness(undefined);

    await client.resume().catch(() => undefined);

    expect(calls.length).toBeGreaterThan(0);
    expect(calls[0].headers).not.toHaveProperty("X-Acting-Tenant-Id");
  });
});
