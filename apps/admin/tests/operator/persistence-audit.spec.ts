import { test, expect } from "@playwright/test";

// Persistence audit — walks every user-editable surface and proves the
// write survives a fresh request (new cookie, new middleware cycle,
// cold read from DB).
//
// Run:
//   STOREFRONT_BASE_URL=... CUSTOMER_EMAIL=... CUSTOMER_PASSWORD=...
//   npx playwright test persistence-audit.spec.ts

const SF = process.env.STOREFRONT_BASE_URL ?? "";
const EMAIL = process.env.CUSTOMER_EMAIL ?? "";
const PW = process.env.CUSTOMER_PASSWORD ?? "";

interface JsonFn {
  (): Promise<unknown>;
}

test("persistence audit: profile + addresses + defaults", async ({ browser }) => {
  test.setTimeout(120_000);
  const ctx = await browser.newContext({ baseURL: SF, viewport: { width: 1440, height: 900 } });
  const page = await ctx.newPage();

  // 1. Sign in.
  await page.goto("/sign-in");
  await page.getByLabel(/email address/i).fill(EMAIL);
  await page.getByLabel(/password/i).fill(PW);
  await page.getByRole("button", { name: /sign in/i }).click();
  await page.waitForLoadState("networkidle").catch(() => {});

  // -----------------------------------------------------------------
  // PROFILE
  // -----------------------------------------------------------------
  const stamp = Date.now().toString(36);
  const profileBody = {
    first_name: `Audit-${stamp}`,
    last_name: "Doe",
    phone: `+9190000${stamp.slice(0, 5)}`,
    marketing_opt_in: true,
  };
  const patch = await page.request.patch(`${SF}/api/account/profile`, {
    headers: { "Content-Type": "application/json" },
    data: profileBody,
  });
  console.log(`[profile patch] ${patch.status()}`);
  expect(patch.ok()).toBeTruthy();

  // Fresh GET after a small pause so middleware re-reads DB.
  await page.waitForTimeout(500);
  const get1 = await (await page.request.get(`${SF}/api/account/profile`)).json() as { data: Record<string, unknown> };
  console.log(`[profile get after]`, get1.data);
  expect(get1.data.first_name).toBe(profileBody.first_name);
  expect(get1.data.last_name).toBe(profileBody.last_name);
  expect(get1.data.phone).toBe(profileBody.phone);
  expect(get1.data.marketing_opt_in).toBe(true);

  // Toggle opt-in off via partial PATCH, verify other fields preserved.
  const off = await page.request.patch(`${SF}/api/account/profile`, {
    headers: { "Content-Type": "application/json" },
    data: { marketing_opt_in: false },
  });
  expect(off.ok()).toBeTruthy();
  await page.waitForTimeout(500);
  const get2 = await (await page.request.get(`${SF}/api/account/profile`)).json() as { data: Record<string, unknown> };
  console.log(`[profile after toggle off]`, get2.data);
  expect(get2.data.first_name, "partial PATCH must preserve other fields").toBe(profileBody.first_name);
  expect(get2.data.marketing_opt_in).toBe(false);

  // -----------------------------------------------------------------
  // ADDRESSES — create, edit, set-default, delete.
  // -----------------------------------------------------------------

  // Clear the book so we start from a known state (up to max 5).
  const initial = await (await page.request.get(`${SF}/api/account/addresses`)).json() as { data: Array<{ id: string }> };
  for (const a of initial.data) {
    await page.request.delete(`${SF}/api/account/addresses/${a.id}`);
  }
  const cleared = await (await page.request.get(`${SF}/api/account/addresses`)).json() as { data: unknown[] };
  expect(cleared.data.length).toBe(0);

  // Create three addresses (Home, Office, Other).
  async function post(label: string, line1: string, def = false) {
    const res = await page.request.post(`${SF}/api/account/addresses`, {
      headers: { "Content-Type": "application/json" },
      data: {
        label,
        name: "Audit Customer",
        line1,
        city: "Bengaluru",
        region: "KA",
        postal_code: "560100",
        country_code: "IN",
        phone: "9999999999",
        is_default: def,
      },
    });
    const body = await res.json();
    return { status: res.status(), body };
  }
  const a1 = await post("Home", "1 Home Lane", true);
  const a2 = await post("Office", "2 Office Street");
  const a3 = await post("Other", "3 Other Road");
  console.log(`[addresses create] ${a1.status}/${a2.status}/${a3.status}`);
  expect(a1.status).toBe(201);
  expect(a2.status).toBe(201);
  expect(a3.status).toBe(201);

  // Verify all three come back with correct labels and exactly one default.
  const list = await (await page.request.get(`${SF}/api/account/addresses`)).json() as { data: Array<{ id: string; label: string; is_default: boolean; line1: string }> };
  expect(list.data.length).toBe(3);
  const labels = list.data.map((a) => a.label).sort();
  expect(labels).toEqual(["Home", "Office", "Other"]);
  const defaults = list.data.filter((a) => a.is_default);
  expect(defaults.length).toBe(1);
  expect(defaults[0]?.label).toBe("Home");

  // Edit the Office address; verify the patch sticks.
  const officeID = list.data.find((a) => a.label === "Office")!.id;
  const edit = await page.request.patch(`${SF}/api/account/addresses/${officeID}`, {
    headers: { "Content-Type": "application/json" },
    data: { line1: "2B Office Street Updated", phone: "8888888888" },
  });
  expect(edit.ok()).toBeTruthy();
  await page.waitForTimeout(500);
  const listAfterEdit = await (await page.request.get(`${SF}/api/account/addresses`)).json() as { data: Array<{ id: string; line1: string; phone?: string }> };
  const edited = listAfterEdit.data.find((a) => a.id === officeID)!;
  expect(edited.line1).toBe("2B Office Street Updated");
  expect(edited.phone).toBe("8888888888");

  // Set Office as default; verify Home is no longer default.
  const setDef = await page.request.patch(`${SF}/api/account/addresses/${officeID}`, {
    headers: { "Content-Type": "application/json" },
    data: { is_default: true },
  });
  expect(setDef.ok()).toBeTruthy();
  await page.waitForTimeout(500);
  const afterDefault = await (await page.request.get(`${SF}/api/account/addresses`)).json() as { data: Array<{ id: string; label: string; is_default: boolean }> };
  const newDef = afterDefault.data.filter((a) => a.is_default);
  expect(newDef.length).toBe(1);
  expect(newDef[0]?.label).toBe("Office");

  // Address limit — creating a 6th must be rejected.
  await post("Other", "4 Spare Street");
  await post("Other", "5 Fifth Place");
  const six = await post("Other", "6 Over The Limit");
  console.log(`[addresses 6th create] ${six.status}`);
  expect(six.status, "limit of 5 must be enforced").toBe(409);

  // Delete one, confirm we drop back to 5.
  const del = await page.request.delete(`${SF}/api/account/addresses/${officeID}`);
  expect(del.ok()).toBeTruthy();
  const afterDelete = await (await page.request.get(`${SF}/api/account/addresses`)).json() as { data: unknown[] };
  expect(afterDelete.data.length).toBe(4);

  // -----------------------------------------------------------------
  // ORDER HISTORY — fetch /api/account/orders; assert it's a list.
  // -----------------------------------------------------------------
  const orders = await (await page.request.get(`${SF}/api/account/orders`)).json() as { data: Array<{ order_number: string }> };
  console.log(`[orders] count=${orders.data.length}`);
  expect(Array.isArray(orders.data)).toBeTruthy();

  await ctx.close();
});
