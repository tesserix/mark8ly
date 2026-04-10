import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { createApiClient } from "../client";
import { ApiError } from "../types";

describe("createApiClient", () => {
  const baseUrl = "https://api.mark8ly.com";
  const storeSlug = "test-store";

  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("constructs the correct URL for a GET request", async () => {
    (fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ data: [] }),
    });

    const client = createApiClient({ baseUrl, storeSlug });
    await client.get("/products");

    expect(fetch).toHaveBeenCalledWith(
      `${baseUrl}/api/v1/mobile/storefront/stores/${storeSlug}/products`,
      expect.objectContaining({ method: "GET" })
    );
  });

  it("constructs the correct URL for POST with body", async () => {
    (fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ data: { id: "ord_1" } }),
    });

    const client = createApiClient({ baseUrl, storeSlug });
    const body = { lineItems: [] };
    await client.post("/checkout", body);

    expect(fetch).toHaveBeenCalledWith(
      `${baseUrl}/api/v1/mobile/storefront/stores/${storeSlug}/checkout`,
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify(body),
      })
    );
  });

  it("attaches Authorization Bearer header when getToken returns a token", async () => {
    (fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ data: {} }),
    });

    const token = "my-jwt-token";
    const getToken = vi.fn().mockResolvedValue(token);
    const client = createApiClient({ baseUrl, storeSlug, getToken });
    await client.get("/account/profile");

    const callArgs = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    const options = callArgs?.[1] as RequestInit;
    expect((options.headers as Record<string, string>)["Authorization"]).toBe(
      `Bearer ${token}`
    );
  });

  it("does NOT attach Authorization header when getToken returns null", async () => {
    (fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ data: {} }),
    });

    const getToken = vi.fn().mockResolvedValue(null);
    const client = createApiClient({ baseUrl, storeSlug, getToken });
    await client.get("/products");

    const callArgs = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    const options = callArgs?.[1] as RequestInit;
    const headers = options.headers as Record<string, string>;
    expect(headers["Authorization"]).toBeUndefined();
  });

  it("does NOT attach Authorization header when getToken is not provided", async () => {
    (fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ data: [] }),
    });

    const client = createApiClient({ baseUrl, storeSlug });
    await client.get("/products");

    const callArgs = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    const options = callArgs?.[1] as RequestInit;
    const headers = options.headers as Record<string, string>;
    expect(headers["Authorization"]).toBeUndefined();
  });

  it("throws ApiError with status code and body on non-ok response", async () => {
    const errorBody = { error: "NOT_FOUND", message: "Product not found" };
    const mockFetch = fetch as ReturnType<typeof vi.fn>;
    mockFetch.mockResolvedValue({
      ok: false,
      status: 404,
      json: async () => errorBody,
    });

    const client = createApiClient({ baseUrl, storeSlug });
    await expect(client.get("/products/nonexistent")).rejects.toThrow(ApiError);
    await expect(client.get("/products/nonexistent")).rejects.toMatchObject({
      statusCode: 404,
      body: errorBody,
    });
  });

  it("throws ApiError with 500 on network failure", async () => {
    (fetch as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new Error("Network error")
    );

    const client = createApiClient({ baseUrl, storeSlug });
    await expect(client.get("/products")).rejects.toThrow(ApiError);
  });

  it("supports PUT, PATCH, DELETE, and HEAD methods", async () => {
    const okResponse = {
      ok: true,
      json: async () => ({ data: {} }),
    };

    const mockFetch = fetch as ReturnType<typeof vi.fn>;

    mockFetch.mockResolvedValueOnce(okResponse);
    const client = createApiClient({ baseUrl, storeSlug });
    await client.put("/account/profile", { displayName: "Alice" });
    expect(mockFetch.mock.calls[0]?.[1]).toMatchObject({ method: "PUT" });

    mockFetch.mockResolvedValueOnce(okResponse);
    await client.patch("/account/profile", { phone: "+1234567890" });
    expect(mockFetch.mock.calls[1]?.[1]).toMatchObject({ method: "PATCH" });

    mockFetch.mockResolvedValueOnce({ ok: true, json: async () => ({}) });
    await client.delete("/wishlist/item-1");
    expect(mockFetch.mock.calls[2]?.[1]).toMatchObject({ method: "DELETE" });

    mockFetch.mockResolvedValueOnce({ ok: true, json: async () => ({}) });
    await client.head("/products");
    expect(mockFetch.mock.calls[3]?.[1]).toMatchObject({ method: "HEAD" });
  });

  it("includes Content-Type: application/json on requests with a body", async () => {
    (fetch as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ data: {} }),
    });

    const client = createApiClient({ baseUrl, storeSlug });
    await client.post("/checkout", { lineItems: [] });

    const callArgs = (fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    const options = callArgs?.[1] as RequestInit;
    const headers = options.headers as Record<string, string>;
    expect(headers["Content-Type"]).toBe("application/json");
  });
});
