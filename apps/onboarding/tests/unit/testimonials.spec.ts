import { test, expect } from "@playwright/test";
import { loadTestimonials } from "../../lib/testimonials";

const realFetch = globalThis.fetch;
test.afterEach(() => {
  globalThis.fetch = realFetch;
});

test("loads public reviews without replacing an honest lower rating", async () => {
  globalThis.fetch = async () =>
    new Response(
      JSON.stringify({
        data: [
          {
            store_id: "s1",
            store_name: "A shop",
            position: "Store owner",
            quote: "Helpful.",
            rating: 3,
          },
        ],
      }),
    );
  expect(await loadTestimonials("http://platform.test")).toEqual([
    {
      store_id: "s1",
      store_name: "A shop",
      position: "Store owner",
      quote: "Helpful.",
      rating: 3,
    },
  ]);
});

test("never substitutes seed reviews when the database is empty or unavailable", async () => {
  globalThis.fetch = async () => new Response(JSON.stringify({ data: [] }));
  expect(await loadTestimonials("http://platform.test")).toEqual([]);
  globalThis.fetch = async () => new Response("unavailable", { status: 503 });
  await expect(loadTestimonials("http://platform.test")).rejects.toThrow();
});

test("rejects malformed review data", async () => {
  globalThis.fetch = async () =>
    new Response(JSON.stringify({ data: [{ rating: 6 }] }));
  await expect(loadTestimonials("http://platform.test")).rejects.toThrow();
});
