import { expect, test } from "@playwright/test";

import {
  ADMIN_URL,
  completeOnboarding,
  fetchPlatformStoreId,
  writeFgaTuples,
} from "./helpers";

const AUTH_BFF_URL = process.env.AUTH_BFF_URL ?? "http://localhost:8087";

/**
 * Phase O — Role-aware settings gate.
 *
 * There is no invite-teammate UI yet, so this spec seeds the "viewer"
 * role tuple directly against OpenFGA. The scenario:
 *
 *   1. Onboard a merchant, sign them in to admin. They arrive as
 *      `owner` via the onboarding outbox FGA write.
 *   2. Grab their GIP UID from auth-bff /auth/session.
 *   3. In OpenFGA: delete the `owner` tuple, write a `viewer` tuple
 *      for the same (user, tenant) pair. Atomic so there's no
 *      milliseconds-long window where the user has zero roles and
 *      middleware boots them to /login.
 *   4. Reload /settings/general. The role badge in the header now
 *      says "viewer", the page renders a read-only banner, the
 *      store-name input is disabled, and the Save / Reset buttons
 *      are gone.
 *   5. As defense-in-depth, the server action still enforces —
 *      Phase N's writeup called this out explicitly. We don't
 *      directly test the forbidden path in this spec because the
 *      button isn't rendered; the backend FGA check is exercised
 *      by the unit tests in tenant/handler_test.go (TODO) and by
 *      the Go-level FakeClient tests in authz/fake_test.go.
 */
test("viewer sees read-only settings page after role change", async ({
  browser,
  request,
}) => {
  // ── 1. Onboard + sign in ─────────────────────────────────────────
  const signupCtx = await browser.newContext();
  const signupPage = await signupCtx.newPage();
  const details = await completeOnboarding(signupPage, request, "role-gate");
  await signupCtx.close();

  const ctx = await browser.newContext();
  const page = await ctx.newPage();

  await page.goto(`${ADMIN_URL}/login`);
  await page.getByLabel(/email address/i).fill(details.email);
  await page.getByLabel(/password/i).fill(details.password);
  await page.getByRole("button", { name: /^sign in$/i }).click();
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 15_000 });

  // Owner path: role badge says "owner".
  await expect(page.getByTestId("role-badge")).toHaveText(/owner/i);

  // ── 2. Pull the uid + tenant_id from auth-bff /auth/session ─────
  // Forward the browser's cookies via Playwright's request context.
  const cookies = await ctx.cookies();
  const cookieHeader = cookies
    .map((c) => `${c.name}=${c.value}`)
    .join("; ");
  const sessionRes = await request.get(`${AUTH_BFF_URL}/auth/session`, {
    headers: { Cookie: cookieHeader },
  });
  expect(sessionRes.ok(), "auth-bff /auth/session").toBeTruthy();
  const sessionBody = (await sessionRes.json()) as {
    data: { user_id: string; tenant_id: string };
  };
  const { user_id: uid, tenant_id: tenantId } = sessionBody.data;

  // ── 3. Demote owner → viewer in one atomic OpenFGA write ────────
  const storeId = await fetchPlatformStoreId(request);
  await writeFgaTuples(request, storeId, {
    writes: [
      {
        user: `user:${uid}`,
        relation: "viewer",
        object: `tenant:${tenantId}`,
      },
    ],
    deletes: [
      {
        user: `user:${uid}`,
        relation: "owner",
        object: `tenant:${tenantId}`,
      },
    ],
  });

  // ── 4. Navigate to settings — now read-only ────────────────────
  // `/settings/general` merged into `/settings/stores` during the
  // IA restructure; the store identity form lives on the stores page.
  await page.goto(`${ADMIN_URL}/settings/stores`);
  await expect(page.getByTestId("role-badge")).toHaveText(/viewer/i);
  await expect(page.getByText(/read-only:/i)).toBeVisible();

  // Store-name input is disabled.
  await expect(page.getByLabel("Store name")).toBeDisabled();

  // Save / Reset buttons are gone — the form hides them when editable=false.
  await expect(
    page.getByRole("button", { name: /save changes/i }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: /^reset$/i }),
  ).toHaveCount(0);

  await ctx.close();
});
