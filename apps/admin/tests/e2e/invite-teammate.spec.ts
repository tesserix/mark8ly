import { expect, test } from "@playwright/test";

import {
  ADMIN_URL,
  API_URL,
  completeOnboarding,
  uniqueEmail,
} from "./helpers";

/**
 * Phase P — Invite teammate end-to-end.
 *
 * Covers the happy path across the whole flow:
 *
 *   1. Onboard tenant A's owner.
 *   2. Sign in, navigate to /settings/team.
 *   3. Invite a fresh email B as role=viewer.
 *   4. Fetch the plaintext invitation token via the dev test helper
 *      (bypasses the inbox).
 *   5. Open /accept-invite?token=… in a fresh browser context with
 *      no session cookie.
 *   6. Pick the "create new account" path, submit a password.
 *   7. Accept flow: accept endpoint writes the FGA tuple, auto-login
 *      mints the session cookie, lands on /dashboard of tenant A.
 *   8. Navigate to /settings/general — assert the Phase O read-only
 *      banner is visible (B is a viewer, not an editor).
 *
 * This one spec is the regression guard for every Phase P moving
 * piece: invitation create + FGA can_invite_members gate, test
 * helper endpoint, token verify, strict email match in accept,
 * WriteRole into FGA, session mint, cookie forwarding, and Phase O's
 * read-only gate still honouring the new role.
 */
test("owner invites viewer → invitee accepts via password → lands in read-only mode", async ({
  browser,
  request,
}) => {
  // ── 1. Onboard tenant A as owner ────────────────────────────────
  const signupCtx = await browser.newContext();
  const signupPage = await signupCtx.newPage();
  const owner = await completeOnboarding(signupPage, request, "invite-owner");
  await signupCtx.close();

  // ── 2. Sign in + navigate to /settings/team ─────────────────────
  const ownerCtx = await browser.newContext();
  const ownerPage = await ownerCtx.newPage();

  await ownerPage.goto(`${ADMIN_URL}/login`);
  await ownerPage.getByLabel(/email address/i).fill(owner.email);
  await ownerPage.getByLabel(/password/i).fill(owner.password);
  await ownerPage.getByRole("button", { name: /^sign in$/i }).click();
  await expect(ownerPage).toHaveURL(/\/dashboard/, { timeout: 15_000 });

  await ownerPage.goto(`${ADMIN_URL}/settings/team`);
  await expect(
    ownerPage.getByRole("heading", { name: /^team$/i }),
  ).toBeVisible();

  // ── 3. Invite a fresh email as viewer ───────────────────────────
  const invitee = uniqueEmail("invite-viewer");
  await ownerPage.getByLabel("Email").fill(invitee.email);
  // Role picker is a radix Select combobox (not a native <select>);
  // click the trigger and pick the option from the popup.
  await ownerPage.getByLabel("Role").click();
  await ownerPage.getByRole("option", { name: /viewer/i }).click();
  await ownerPage.getByRole("button", { name: /send invite/i }).click();
  await expect(
    ownerPage.getByText(new RegExp(`invitation sent to ${invitee.email}`, "i")),
  ).toBeVisible({ timeout: 10_000 });
  await ownerCtx.close();

  // ── 4. Fetch the plaintext token from the dev helper ────────────
  const tokenRes = await request.get(
    `${API_URL}/api/v1/test/invitations/latest?email=${encodeURIComponent(invitee.email)}`,
  );
  expect(tokenRes.ok(), "test helper returned non-2xx").toBeTruthy();
  const tokenBody = (await tokenRes.json()) as { data: { token: string } };
  const token = tokenBody.data.token;
  expect(token, "empty token from test helper").toBeTruthy();

  // ── 5. Fresh browser context, open the accept link ──────────────
  const inviteeCtx = await browser.newContext();
  const inviteePage = await inviteeCtx.newPage();
  await inviteePage.goto(
    `${ADMIN_URL}/accept-invite?token=${encodeURIComponent(token)}`,
  );
  // Wait for the accept form to render. Multiple headings mention
  // the tenant name (hero + form) so we assert on the form's
  // "Confirm password"-eligible mode picker instead.
  await expect(
    inviteePage.getByRole("button", { name: /create a new account/i }),
  ).toBeVisible({ timeout: 10_000 });

  // ── 6. Pick the new-account path, submit a password ────────────
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

  // ── 7. Land on tenant A's dashboard ─────────────────────────────
  await expect(inviteePage).toHaveURL(/\/dashboard/, { timeout: 15_000 });
  // Role badge is the reliable indicator — it's a single element
  // with a data-testid. Viewer role proves the FGA tuple + session
  // mint + middleware role fetch all worked.
  await expect(inviteePage.getByTestId("role-badge")).toHaveText(/viewer/i, {
    timeout: 10_000,
  });

  // ── 8. Settings page shows the read-only banner for the viewer ──
  // `/settings/general` was merged into `/settings/stores` during the
  // IA restructure; the old route redirects.
  await inviteePage.goto(`${ADMIN_URL}/settings/stores`);
  await expect(inviteePage.getByTestId("role-badge")).toHaveText(/viewer/i);
  await expect(inviteePage.getByText(/read-only:/i)).toBeVisible();
  await expect(inviteePage.getByLabel("Store name")).toBeDisabled();

  await inviteeCtx.close();
});
