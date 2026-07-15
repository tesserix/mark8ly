import { z } from "zod";
import { createApiClient, ApiError } from "@repo/mobile-shared/api/client";

function jsonResponse(body: unknown): Response {
  return { status: 200, ok: true, json: async () => body, text: async () => "" } as Response;
}

describe("contract mismatch", () => {
  const realFetch = globalThis.fetch;
  afterEach(() => {
    globalThis.fetch = realFetch;
  });

  function clientReturning(body: unknown) {
    globalThis.fetch = jest.fn().mockResolvedValue(jsonResponse(body)) as unknown as typeof fetch;
    return createApiClient({
      baseUrl: "https://x.test",
      getToken: async () => "t",
      getStoreId: () => null,
    });
  }

  const schema = z.object({
    top_products: z.array(z.object({ units_sold: z.number() })),
  });

  it("throws ApiError with code contract_mismatch", async () => {
    const client = clientReturning({ top_products: [{}] });
    await expect(client.get("/dashboard", undefined, schema)).rejects.toBeInstanceOf(ApiError);
  });

  it("names the exact field path in the message", async () => {
    const client = clientReturning({ top_products: [{}] });
    // Silent `undefined` is what let 31 mismatches hide for two months. The
    // message must point at the field, not just say "invalid".
    await expect(client.get("/dashboard", undefined, schema)).rejects.toMatchObject({
      status: 500,
      code: "contract_mismatch",
      message: "top_products.0.units_sold: Invalid input: expected number, received undefined",
    });
  });

  it("returns parsed data when the schema matches", async () => {
    const client = clientReturning({ top_products: [{ units_sold: 2 }] });
    await expect(client.get("/dashboard", undefined, schema)).resolves.toEqual({
      top_products: [{ units_sold: 2 }],
    });
  });
});
