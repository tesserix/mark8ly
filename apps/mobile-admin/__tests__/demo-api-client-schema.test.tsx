import { z } from "zod";
import { createDemoApiClient } from "@/lib/demo-api-client";
import { ApiError } from "@repo/mobile-shared/api/client";

describe("createDemoApiClient — schema application", () => {
  it("applies a passed schema and returns the parsed value", async () => {
    const client = createDemoApiClient();
    const schema = z.object({ data: z.array(z.object({ id: z.string() })) });
    const res = await client.get("/stores", undefined, schema);
    expect(res.data[0]!.id).toBe("demo-store");
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
