import { afterEach, describe, expect, it, vi } from "vitest";
import { tenantIdForHostSlug } from "./tenant-host";

function stubFetch(status: number, body: unknown) {
  const fn = vi.fn().mockResolvedValue(
    new Response(JSON.stringify(body), {
      status,
      headers: { "content-type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", fn);
  return fn;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("tenantIdForHostSlug", () => {
  it("returns null for a falsy host", async () => {
    expect(await tenantIdForHostSlug(null)).toBeNull();
    expect(await tenantIdForHostSlug(undefined)).toBeNull();
    expect(await tenantIdForHostSlug("")).toBeNull();
  });

  it("returns null for the bare admin host (no tenant slug)", async () => {
    expect(await tenantIdForHostSlug("admin.mark8ly.com")).toBeNull();
  });

  it("resolves a tenant id for a per-tenant admin subdomain", async () => {
    const fetchMock = stubFetch(200, { data: { tenant_id: "tenant-1" } });

    const result = await tenantIdForHostSlug("demo-store-admin.mark8ly.com");

    expect(result).toBe("tenant-1");
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain("/internal/stores/by-slug/demo-store");
  });

  it("strips a port before matching", async () => {
    stubFetch(200, { data: { tenant_id: "tenant-1" } });

    const result = await tenantIdForHostSlug("demo-store-admin.mark8ly.com:3000");

    expect(result).toBe("tenant-1");
  });

  it("returns null when platform-api 404s the slug", async () => {
    stubFetch(404, { error: "not_found" });

    expect(await tenantIdForHostSlug("unknown-store-admin.mark8ly.com")).toBeNull();
  });

  it("returns null (never throws) when platform-api is unreachable", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("network down")));

    await expect(tenantIdForHostSlug("demo-store-admin.mark8ly.com")).resolves.toBeNull();
  });
});
