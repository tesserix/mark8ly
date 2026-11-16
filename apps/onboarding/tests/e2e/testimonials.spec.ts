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
            position: "Store owner",
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
      section.getByText("Store owner", { exact: true }),
    ).toBeVisible();
    await expect(section.getByText("Test Seller", { exact: true })).toHaveCount(
      0,
    );
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

const fourReviews = ["One", "Two", "Three", "Four"].map((name, i) => ({
  store_id: `shop-${i + 1}`,
  store_name: `${name} shop`,
  position: "Store owner",
  quote: `Review from shop ${name}.`,
  rating: 5,
}));

async function openCarousel(
  page: import("@playwright/test").Page,
  width: number,
) {
  await page.clock.install();
  await page.route("**/api/testimonials", (route) =>
    route.fulfill({ json: { data: fourReviews } }),
  );
  await page.setViewportSize({ width, height: 900 });
  await page.goto("/");
  const section = page.getByRole("region", { name: "In their own words." });
  await section.scrollIntoViewIfNeeded();
  await page.clock.pauseAt(Date.now() + 1_000);
  const slide = (n: number) =>
    section.getByRole("group", { name: `${n} of 4` });
  return { section, slide };
}

test("testimonials rest on each review, then slide left to the next", async ({
  page,
}) => {
  const { section, slide } = await openCarousel(page, 1280);
  const followingSection = await section.evaluate(
    (el) => el.nextElementSibling?.querySelector("h2")?.textContent,
  );
  expect(followingSection).toContain("See it move.");
  await expect(slide(1)).toBeInViewport({ ratio: 0.9 });
  await expect(slide(4)).not.toBeInViewport();
  await page.clock.runFor(5_000);
  await expect(slide(4)).toBeInViewport({ ratio: 0.9 });
  await expect(slide(1)).not.toBeInViewport();
  await section.getByRole("button", { name: "Pause testimonials" }).click();
  await page.mouse.move(0, 0);
  await page.evaluate(() => (document.activeElement as HTMLElement).blur());
  await page.clock.runFor(20_000);
  await expect(slide(4)).toBeInViewport({ ratio: 0.9 });
  await expect(
    section.getByRole("button", { name: "Play testimonials" }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
});

test("phone readers step through testimonials and wrap around", async ({
  page,
}) => {
  const { section, slide } = await openCarousel(page, 390);
  await expect(slide(1)).toBeInViewport({ ratio: 0.9 });
  await expect(slide(2)).not.toBeInViewport({ ratio: 0.9 });
  await section.getByRole("button", { name: "Next testimonial" }).click();
  await expect(slide(2)).toBeInViewport({ ratio: 0.9 });
  await section.getByRole("button", { name: "Previous testimonial" }).click();
  await expect(slide(1)).toBeInViewport({ ratio: 0.9 });
  await section.getByRole("button", { name: "Previous testimonial" }).click();
  await page.clock.runFor(100);
  await expect(slide(4)).toBeInViewport({ ratio: 0.9 });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
});

test("testimonials never move on their own for reduced-motion readers", async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  const { section, slide } = await openCarousel(page, 1280);
  await expect(slide(1)).toBeInViewport({ ratio: 0.9 });
  await page.clock.runFor(20_000);
  await expect(slide(1)).toBeInViewport({ ratio: 0.9 });
  await expect(
    section.getByRole("button", { name: "Pause testimonials" }),
  ).toHaveCount(0);
  await section.getByRole("button", { name: "Next testimonial" }).click();
  await expect(slide(4)).toBeInViewport({ ratio: 0.9 });
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
