import { z } from "zod";
import { createDemoApiClient } from "@/lib/demo-api-client";
import { ApiError } from "@repo/mobile-shared/api/client";
import {
  productDetailSchema,
  productListSchema,
} from "@repo/mobile-shared/api/schemas/products";
import { primaryVariant } from "@/lib/product-display";

describe("createDemoApiClient — schema application", () => {
  it("applies a passed schema and returns the parsed value", async () => {
    const client = createDemoApiClient();
    const schema = z.object({ data: z.array(z.object({ id: z.string() })) });
    const res = await client.get("/stores", undefined, schema);
    expect(res.data[0]!.id).toBe("demo-store");
  });

  it("returns the schema's transformed output, not the raw fixture", async () => {
    const client = createDemoApiClient();
    const schema = z.object({
      data: z.array(z.object({ id: z.string().transform((s) => s.toUpperCase()) })),
    });
    const res = await client.get("/stores", undefined, schema);
    // Raw fixture id is "demo-store"; only the parsed/transformed output is uppercased.
    expect(res.data[0]!.id).toBe("DEMO-STORE");
  });

  it("throws contract_mismatch naming the field path when the fixture does not match", async () => {
    const client = createDemoApiClient();
    // /stores fixture has no `nope` key — the schema must reject it.
    const schema = z.object({ nope: z.string() });
    await expect(client.get("/stores", undefined, schema)).rejects.toMatchObject({
      name: "ApiError",
      status: 500,
      code: "contract_mismatch",
    });
    await expect(client.get("/stores", undefined, schema)).rejects.toThrow(/nope/);
  });

  it("returns raw data unchanged when no schema is passed", async () => {
    const client = createDemoApiClient();
    const res = await client.get<{ data: unknown[] }>("/stores");
    expect(Array.isArray(res.data)).toBe(true);
  });

  it("exposes ApiError as a real error instance", async () => {
    const client = createDemoApiClient();
    const schema = z.object({ nope: z.string() });
    await expect(client.get("/stores", undefined, schema)).rejects.toBeInstanceOf(ApiError);
  });
});

describe("createDemoApiClient — product fixtures match the real wire schema", () => {
  it("serves /products as the real {data, meta} envelope", async () => {
    const client = createDemoApiClient();
    const res = await client.get("/products", undefined, productListSchema);
    expect(res.data).toHaveLength(3);
    expect(res.meta.total).toBe(3);
  });

  it("serves /products/:id as a bare product object", async () => {
    const client = createDemoApiClient();
    const res = await client.get("/products/p-2", undefined, productDetailSchema);
    expect(res.title).toBe("Merino Beanie");
  });

  it("keeps a multi-variant fixture whose positions are OUT OF ORDER, like prod", async () => {
    const client = createDemoApiClient();
    const res = await client.get("/products", undefined, productListSchema);
    const shirt = res.data.find((p) => p.id === "p-1")!;
    // Array order must NOT match position order, or the fixture cannot catch a
    // regression back to `variants[0]`.
    expect(shirt.variants[0]!.position).not.toBe(0);
    expect(primaryVariant(shirt)!.sku).toBe("LCS-001-S");
  });
});

// Regression guard for the demo product Save/Create bug: create/update pass
// productDetailSchema to validate the RESPONSE, but a CreateProductBody /
// UpdateProductBody request has none of id/store_id/handle/media/created_at.
// Echoing the request body back (as the generic mutation path does for every
// other endpoint) made every demo product Save silently throw
// contract_mismatch. POST/PATCH /products must resolve to a real fixture.
describe("createDemoApiClient — product mutations satisfy productDetailSchema", () => {
  it("POST /products resolves to a real product fixture, not an echo of the request body", async () => {
    const client = createDemoApiClient();
    const body = { title: "Brand New Thing", variants: [{ sku: "BNT-1", price: 10 }] };
    const res = await client.post("/products", body, productDetailSchema);
    // If this were an echo, `store_id`/`handle`/`media`/`created_at` would be
    // missing and productDetailSchema would already have thrown above.
    expect(res.id).toBeTruthy();
    expect(res.store_id).toBe("demo-store");
  });

  it("PATCH /products/:id resolves to the matching product fixture, not an echo of the request body", async () => {
    const client = createDemoApiClient();
    const res = await client.patch("/products/p-2", { title: "Renamed Beanie" }, productDetailSchema);
    expect(res.id).toBe("p-2");
    // The response is the canned fixture, not the request body's title.
    expect(res.title).toBe("Merino Beanie");
  });

  it("PATCH /products/:unknown-id falls back to the first product fixture", async () => {
    const client = createDemoApiClient();
    const res = await client.patch("/products/does-not-exist", { title: "X" }, productDetailSchema);
    expect(res.id).toBe("p-1");
  });
});
