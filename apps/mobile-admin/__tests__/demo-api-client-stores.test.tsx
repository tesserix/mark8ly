import { createDemoApiClient } from "@/lib/demo-api-client";
import { storesResponseSchema } from "@repo/mobile-shared/api/schemas/stores";

// Regression guard: Task 5 switched useStores() from Array.isArray(data)
// handling to `res.data` on the real {data:[...]} envelope, but the demo
// backend still returned a bare array — undefined `res.data` crashed
// react-query in EXPO_PUBLIC_AUTH_BACKEND=demo builds. The demo client must
// mirror the real wire shape for every endpoint the hooks parse with a
// schema.
describe("createDemoApiClient /stores", () => {
  it("returns the {data:[...]} envelope, not a bare array", async () => {
    const client = createDemoApiClient();
    const res = await client.getTenant("/stores");

    expect(Array.isArray(res)).toBe(false);
    expect(Array.isArray((res as { data: unknown[] }).data)).toBe(true);
    expect((res as { data: unknown[] }).data).toHaveLength(1);
  });

  it("parses under the real storesResponseSchema used by useStores()", async () => {
    const client = createDemoApiClient();
    const res = await client.getTenant("/stores");

    const parsed = storesResponseSchema.parse(res);
    expect(parsed.data).toHaveLength(1);
  });
});
