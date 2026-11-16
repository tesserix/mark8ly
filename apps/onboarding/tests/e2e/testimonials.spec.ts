import { test, expect } from "@playwright/test";

test("homepage renders submitted words and ratings at desktop and phone widths", async ({
  page,
}) => {
  await page.route("**/api/testimonials", (route) =>
    route.fulfill({
      json: {
        data: [
          {
            store_id: "shop-1",
            store_name: "Review test shop",
            author_name: "Test Seller",
            quote: "An honest review of setting up my shop.",
            rating: 3,
          },
        ],
      },
    }),
  );
  for (const width of [1280, 390]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto("/");
    const section = page.getByRole("region", { name: "In their own words." });
    await expect(section).toBeVisible();
    await expect(
      section.getByText("Test Seller", { exact: true }),
    ).toBeVisible();
    await expect(
      section.getByText("Online store of Review test shop", { exact: true }),
    ).toBeVisible();
    await expect(
      section.getByRole("img", { name: "3 out of 5 stars" }),
    ).toBeVisible();
    await expect(section.locator("blockquote")).toHaveText(
      "“An honest review of setting up my shop.”",
    );
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
  }
});

test("homepage omits testimonials when there are no published reviews", async ({
  page,
}) => {
  await page.route("**/api/testimonials", (route) =>
    route.fulfill({ json: { data: [] } }),
  );
  const response = page.waitForResponse("**/api/testimonials");
  await page.goto("/");
  await response;
  await expect(
    page.getByRole("region", { name: "In their own words." }),
  ).toHaveCount(0);
});
