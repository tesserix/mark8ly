// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { NextRequest } from "next/server";

vi.mock("next/headers", () => ({ cookies: async () => ({ get: () => undefined }) }));
vi.mock("@/lib/auth/serverSession", () => ({ getServerSessionContext: vi.fn() }));
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { POST } from "../../../app/api/otto-platform/[...path]/route";

afterEach(() => vi.unstubAllGlobals());
describe("platform support product provenance", () => {
 it("stamps mark8ly from the server and ignores forged browser scope", async () => {
  vi.mocked(getServerSessionContext).mockResolvedValue({ userId: "merchant", tenantId: "tenant-a", tenantName: "Example" } as Awaited<ReturnType<typeof getServerSessionContext>>);
  const fetch = vi.fn().mockResolvedValue(new Response('{"conversation":{"id":"case"}}', { status: 201 }));
  vi.stubGlobal("fetch", fetch);
  const response = await POST(new NextRequest("https://admin.mark8ly.com/api/otto-platform/conversations", { method: "POST", headers: { "X-Product-Id": "other-product", "X-Client-Tenant-Id": "victim" }, body: '{"message":"Help"}' }), { params: Promise.resolve({ path: ["conversations"] }) });
  expect(response.status).toBe(201);
  expect(fetch.mock.calls[0][1].headers).toMatchObject({ "X-Product-Id": "mark8ly", "X-Client-Tenant-Id": "tenant-a", "X-User-Id": "merchant" });
 });
 it("rejects unauthenticated callers before reaching Otto", async () => {
  vi.mocked(getServerSessionContext).mockResolvedValue(null);
  const fetch = vi.fn(); vi.stubGlobal("fetch", fetch);
  const response = await POST(new NextRequest("https://admin.mark8ly.com/api/otto-platform/conversations", { method: "POST" }), { params: Promise.resolve({ path: ["conversations"] }) });
  expect(response.status).toBe(401); expect(fetch).not.toHaveBeenCalled();
 });
});
