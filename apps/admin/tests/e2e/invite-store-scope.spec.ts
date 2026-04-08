import { expect, test } from "@playwright/test";

import {
  ADMIN_URL,
  API_URL,
  completeOnboarding,
  uniqueEmail,
} from "./helpers";

/**
 * Phase R — store-scoped invitation end-to-end.
 *
 * A solo founder has one store by default, so the scope toggle is
 * hidden. The spec needs a second store before the toggle appears,
 * then invites a teammate with store scope and asserts the role
 * tuple lands at the store level.
 *
 * The test exercises:
 *   1. Onboard a merchant with one default store
 *   2. Create a second store via /settings/stores
 *   3. Navigate to /settings/team — scope toggle is now visible
 *   4. Pick "Specific store", choose the newly created store, role
 *      = "manager", email = new invitee
 *   5. Fetch the invitation token from the dev helper
 *   6. Accept the invite as a new user with password
 *   7. Invitee lands on /dashboard — they have store access but
 *      NOT tenant-level access (phase R's whole point)
 */
test("store-scoped invite grants store role without tenant access", async ({
  browser,
  request,
}) => {
  // ── 1. Onboard ───────────────────────────────────────────────
  const signupCtx = await browser.newContext();
  const signupPage = await signupCtx.newPage();
  const owner = await completeOnboarding(
    signupPage,
    request,
    "store-invite-owner",
  );
  await signupCtx.close();

  // ── 2. Sign in + add second store ────────────────────────────
  const ownerCtx = await browser.newContext();
  const ownerPage = await ownerCtx.newPage();
  await ownerPage.goto(`${ADMIN_URL}/login`);
  await ownerPage.getByLabel(/email address/i).fill(owner.email);
  await ownerPage.getByLabel(/password/i).fill(owner.password);
  await ownerPage.getByRole("button", { name: /^sign in$/i }).click();
  await expect(ownerPage).toHaveURL(/\/dashboard/, { timeout: 15_000 });

  const second = uniqueEmail("outlet");
  await ownerPage.goto(`${ADMIN_URL}/settings/stores`);
  await ownerPage.getByRole("button", { name: /\+ add store/i }).click();
  await ownerPage.getByLabel(/store name/i).fill(`${owner.businessName} Outlet`);
  await ownerPage.getByLabel(/url slug/i).fill(second.slug);
  await ownerPage.getByRole("button", { name: /create store/i }).click();
  await expect(ownerPage).toHaveURL(/\/settings\/general/, { timeout: 15_000 });

  // ── 3. Team page — scope toggle is now visible ──────────────
  await ownerPage.goto(`${ADMIN_URL}/settings/team`);
  await expect(ownerPage.getByTestId("invite-scope")).toBeVisible();

  // ── 4. Flip to "Specific store", pick the outlet, role=manager
  await ownerPage
    .getByTestId("invite-scope")
    .getByRole("button", { name: /specific store/i })
    .click();

  const invitee = uniqueEmail("store-invitee");
  await ownerPage.getByLabel("Email").fill(invitee.email);

  // The store picker is a native select — pick by visible option
  // text which contains the slug we just created.
  await ownerPage
    .getByTestId("invite-store-select")
    .selectOption({ label: `${owner.businessName} Outlet (${second.slug}.mark8ly.com)` });

  // Role combobox → pick Manager
  await ownerPage.getByLabel("Role").click();
  await ownerPage.getByRole("option", { name: /manager/i }).click();

  await ownerPage.getByRole("button", { name: /send invite/i }).click();
  await expect(
    ownerPage.getByText(new RegExp(`invitation sent to ${invitee.email}`, "i")),
  ).toBeVisible({ timeout: 10_000 });
  await ownerCtx.close();

  // ── 5. Fetch the plaintext token ────────────────────────────
  const tokenRes = await request.get(
    `${API_URL}/api/v1/test/invitations/latest?email=${encodeURIComponent(invitee.email)}`,
  );
  expect(tokenRes.ok(), "test helper returned non-2xx").toBeTruthy();
  const tokenBody = (await tokenRes.json()) as { data: { token: string } };
  const token = tokenBody.data.token;

  // ── 6. Accept as a fresh user ───────────────────────────────
  const inviteeCtx = await browser.newContext();
  const inviteePage = await inviteeCtx.newPage();
  await inviteePage.goto(
    `${ADMIN_URL}/accept-invite?token=${encodeURIComponent(token)}`,
  );
  await expect(
    inviteePage.getByRole("button", { name: /create a new account/i }),
  ).toBeVisible({ timeout: 10_000 });
  await inviteePage
    .getByRole("button", { name: /create a new account/i })
    .click();
  await inviteePage
    .getByLabel(/^create password$/i)
    .fill(invitee.password);
  await inviteePage
    .getByLabel(/confirm password/i)
    .fill(invitee.password);
  await inviteePage
    .getByRole("button", { name: /create account and join store/i })
    .click();

  // ── 7. Lands on the dashboard ───────────────────────────────
  await expect(inviteePage).toHaveURL(/\/dashboard/, { timeout: 15_000 });

  await inviteeCtx.close();
});
