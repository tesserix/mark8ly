import { test } from "@playwright/test";

test("probe profile save", async ({ browser }) => {
  test.setTimeout(60_000);
  const ctx = await browser.newContext({
    baseURL: process.env.STOREFRONT_BASE_URL ?? "",
    viewport: { width: 1440, height: 900 },
  });
  const page = await ctx.newPage();

  // Sign in
  await page.goto("/sign-in");
  await page.getByLabel(/email address/i).fill(process.env.CUSTOMER_EMAIL ?? "");
  await page.getByLabel(/password/i).fill(process.env.CUSTOMER_PASSWORD ?? "");
  await page.getByRole("button", { name: /sign in/i }).click();
  await page.waitForLoadState("networkidle").catch(() => {});

  // GET current profile
  const getRes = await page.request.get(`${process.env.STOREFRONT_BASE_URL}/api/account/profile`);
  console.log(`[profile GET] status=${getRes.status()} body=${(await getRes.text()).slice(0, 400)}`);

  // PATCH profile
  const patchRes = await page.request.patch(`${process.env.STOREFRONT_BASE_URL}/api/account/profile`, {
    headers: { "Content-Type": "application/json" },
    data: {
      first_name: "Probe",
      last_name: "Tester",
      phone: "9999999999",
      marketing_opt_in: true,
    },
  });
  console.log(`[profile PATCH] status=${patchRes.status()} body=${(await patchRes.text()).slice(0, 400)}`);

  // GET again to see if saved
  const verifyRes = await page.request.get(`${process.env.STOREFRONT_BASE_URL}/api/account/profile`);
  console.log(`[profile verify] status=${verifyRes.status()} body=${(await verifyRes.text()).slice(0, 400)}`);

  await ctx.close();
});
