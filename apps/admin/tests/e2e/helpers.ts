import { expect, type APIRequestContext } from "@playwright/test";

/**
 * Role tuple helper used by Phase O specs. There is no invite-teammate
 * UI yet, so specs that need a non-owner role have to reach into the
 * OpenFGA store directly. Takes the store id from `fetchPlatformStoreId`.
 *
 * Writes and deletes are batched so the caller can atomically demote
 * an owner to a viewer (delete owner + write viewer in one call).
 */
export type TenantRole = "owner" | "admin" | "staff" | "viewer";

interface TupleOp {
  user: string; // e.g. "user:<gip-uid>"
  relation: TenantRole;
  object: string; // e.g. "tenant:<uuid>"
}

export async function fetchPlatformStoreId(
  request: APIRequestContext,
): Promise<string> {
  const res = await request.get(`${OPENFGA_URL}/stores`);
  if (!res.ok()) throw new Error(`openfga list stores: ${res.status()}`);
  const body = (await res.json()) as {
    stores: Array<{ id: string; name: string }>;
  };
  const store = body.stores.find((s) => s.name === "mark8ly-platform");
  if (!store) throw new Error("mark8ly-platform store not found in openfga");
  return store.id;
}

export async function writeFgaTuples(
  request: APIRequestContext,
  storeId: string,
  {
    writes = [],
    deletes = [],
  }: { writes?: TupleOp[]; deletes?: TupleOp[] },
): Promise<void> {
  const body: Record<string, unknown> = {};
  if (writes.length > 0) {
    body.writes = {
      tuple_keys: writes.map((w) => ({
        user: w.user,
        relation: w.relation,
        object: w.object,
      })),
    };
  }
  if (deletes.length > 0) {
    body.deletes = {
      tuple_keys: deletes.map((d) => ({
        user: d.user,
        relation: d.relation,
        object: d.object,
      })),
    };
  }
  const res = await request.post(`${OPENFGA_URL}/stores/${storeId}/write`, {
    data: body,
  });
  if (!res.ok()) {
    const text = await res.text();
    throw new Error(`openfga write: ${res.status()} ${text}`);
  }
}

/**
 * Cross-app helpers for the admin e2e suite.
 *
 * The admin tests reuse the same test-helper endpoint on platform-api
 * that the onboarding e2e tests use — `/api/v1/test/verification/latest`
 * — so the admin suite can walk the onboarding golden path without a
 * real inbox.
 */

export const ONBOARDING_URL =
  process.env.ONBOARDING_URL ?? "http://localhost:4201";
export const ADMIN_URL = process.env.ADMIN_URL ?? "http://localhost:4202";
export const API_URL = process.env.API_URL ?? "http://localhost:8086";
// OpenFGA HTTP API. Used by Phase O specs to write role tuples
// directly — there's no "invite teammate" UI yet, so the tests seed
// the tuples the UI would create.
export const OPENFGA_URL = process.env.OPENFGA_URL ?? "http://localhost:8089";
// auth-bff. Only signInAsOwner talks to it directly, via the internal
// mint-session endpoint — see that helper for why.
export const AUTH_BFF_URL = process.env.AUTH_BFF_URL ?? "http://localhost:8087";
// marketplace-api. Used only by seedProducts — see that helper.
export const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8091";

/**
 * Build a unique email + slug + business name per test run so Postgres
 * state from a previous run can't collide.
 */
export function uniqueEmail(label = "admin-e2e"): {
  email: string;
  slug: string;
  businessName: string;
  password: string;
  name: string;
} {
  const stamp = `${Date.now().toString(36)}${Math.floor(
    Math.random() * 1e4,
  ).toString(36)}`;
  const local = `${label}-${stamp}`;
  return {
    email: `${local}@example.com`,
    slug: local.replace(/[^a-z0-9-]/g, "").slice(0, 60),
    businessName: `${label} ${stamp}`,
    // Phase M: the set-password page collects this after the magic link
    // is consumed. Stable per call so the admin sign-in spec can reuse it.
    // Must satisfy the onboarding password policy (>=12 chars with upper,
    // lower, digit AND symbol). The old value had neither an uppercase
    // letter nor a symbol; every completeOnboarding() bounced on the policy
    // message. Same drift #858 stage 3a fixed in the onboarding suite.
    // Deliberately low-entropy: a random-looking literal trips gitleaks.
    password: "E2e-test-password-123!",
    // The set-password step requires a name -- Zitadel needs a
    // givenName/familyName and platform-api splits this single field.
    name: "E2E Tester",
  };
}

/**
 * Poll the test helper endpoint until a magic-link token shows up for
 * the given email.
 */
export async function fetchMagicLinkToken(
  request: APIRequestContext,
  email: string,
  { timeoutMs = 5000 }: { timeoutMs?: number } = {},
): Promise<string> {
  const deadline = Date.now() + timeoutMs;
  let lastStatus = 0;
  while (Date.now() < deadline) {
    const res = await request.get(
      `${API_URL}/api/v1/test/verification/latest?email=${encodeURIComponent(email)}`,
    );
    lastStatus = res.status();
    if (res.ok()) {
      const body = (await res.json()) as { data: { token: string } };
      expect(body.data.token, "test helper returned empty token").toBeTruthy();
      return body.data.token;
    }
    if (res.status() !== 404) {
      throw new Error(`test helper failed: ${res.status()}`);
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error(
    `magic-link token never appeared for ${email} (last status ${lastStatus})`,
  );
}

/**
 * Drives the onboarding form from blank to welcome page in a single
 * function. Returns the generated details so the caller can use them
 * in post-onboarding assertions (e.g. "dashboard shows the business
 * name I just typed").
 *
 * Takes a Playwright `page` already scoped to the onboarding origin —
 * this helper navigates within it rather than creating its own
 * context, so the session cookie it leaves behind is visible to
 * anything else in the same context.
 */
export async function completeOnboarding(
  page: import("@playwright/test").Page,
  request: APIRequestContext,
  label = "admin-e2e",
): Promise<{
  email: string;
  slug: string;
  businessName: string;
  password: string;
  name: string;
}> {
  const details = uniqueEmail(label);

  await page.goto(`${ONBOARDING_URL}/onboarding`);
  await page.getByLabel(/email address/i).fill(details.email);
  await page.getByLabel(/business name/i).fill(details.businessName);
  await page.locator("#slug").fill(details.slug);
  await expect(page.getByText(/✓ available/i)).toBeVisible({ timeout: 5000 });

  await page.getByLabel(/country/i).click();
  await page.getByRole("option", { name: /united states/i }).click();
  const currencyTrigger = page.getByLabel(/currency/i);
  if ((await currencyTrigger.textContent())?.includes("Select")) {
    await currencyTrigger.click();
    await page.getByRole("option", { name: /usd/i }).first().click();
  }
  // "Send verification link", not "get my store ready" — the label
  // changed and this helper had never run to notice. apps/onboarding's
  // own specs already use the current label.
  await page.getByRole("button", { name: /send verification link/i }).click();
  await expect(page).toHaveURL(/\/onboarding\/check-inbox/, {
    timeout: 10_000,
  });

  const token = await fetchMagicLinkToken(request, details.email);
  await page.goto(
    `${ONBOARDING_URL}/onboarding/verify?token=${encodeURIComponent(token)}`,
  );

  // Phase M: verify lands on /onboarding/set-password (not /welcome).
  // Pick the email+password path, submit, then end up on /welcome.
  await expect(page).toHaveURL(/\/onboarding\/set-password/, {
    timeout: 15_000,
  });
  await page.locator("#name").fill(details.name);
  await page.locator("#password").fill(details.password);
  await page.getByRole("button", { name: /create account/i }).click();
  await expect(page).toHaveURL(/\/welcome/, { timeout: 15_000 });

  return details;
}

/**
 * Sign a merchant in WITHOUT driving the login form (#858 stage 3b).
 *
 * The form cannot render on a local stack, and that is deliberate rather
 * than broken: `middleware.ts` 404s canonical `/login` unless it carries a
 * valid `returnUrl`, and `isValidSlugReturnUrl` requires `https:` plus a
 * `{slug}-admin.mark8ly.{com,dev}` host. No local HTTP origin can satisfy
 * both, so for twelve specs the sign-in step is an unreachable prelude to
 * the thing actually under test.
 *
 * So we mint the session directly, the same way break-glass login does —
 * auth-bff's existing `POST /internal/mint-session`, guarded by
 * X-Internal-Auth and an auth_context allow-list. Nothing test-only is
 * added to production, and the cookie is the real one: the same signer,
 * the same shape, validated through the same `/auth/session` round trip.
 *
 * `sign-in.spec.ts` deliberately does NOT use this — it is the spec that
 * tests the login form, so bypassing the form would make it assert nothing.
 *
 * The returned context is pinned to the tenant's slug host. That is not a
 * convenience: on the canonical host an authenticated merchant is bounced
 * to their slug subdomain, and `{slug}-admin.mark8ly.com` is reachable in
 * tests only because playwright.config.ts maps it to 127.0.0.1 (see the
 * `--host-resolver-rules` launch arg there).
 */
export async function signInAsOwner(
  browser: import("@playwright/test").Browser,
  request: APIRequestContext,
  details: { email: string; slug: string },
): Promise<import("@playwright/test").BrowserContext> {
  const secret = process.env.INTERNAL_AUTH_SECRET ?? "";
  if (!secret) {
    throw new Error(
      "INTERNAL_AUTH_SECRET is unset — auth-bff's mint-session endpoint " +
        "fails closed (503 not_configured) without it, and platform-api's " +
        "/internal lookups 401. Set it to the value in infra/dev/docker-compose.yml.",
    );
  }
  const authHeaders = { "X-Internal-Auth": secret };

  // Resolves BOTH ids in one call: the tenant, and owner_user_id — the
  // Zitadel uid platform-api recorded at onboarding. The session must
  // carry the uid, not the email: middleware's role lookup is
  // /internal/tenants/:id/me?uid=.
  const tenantRes = await request.get(
    `${API_URL}/internal/tenants/by-owner-email?email=${encodeURIComponent(details.email)}`,
    { headers: authHeaders },
  );
  expect(
    tenantRes.ok(),
    `by-owner-email failed for ${details.email} (${tenantRes.status()})`,
  ).toBeTruthy();
  const tenant = (await tenantRes.json()).data as {
    id: string;
    owner_user_id: string;
  };

  const mintRes = await request.post(`${AUTH_BFF_URL}/internal/mint-session`, {
    headers: authHeaders,
    data: {
      tenant_id: tenant.id,
      tenant_slug: details.slug,
      user_id: tenant.owner_user_id,
      email: details.email,
      auth_context: "staff",
      ttl_seconds: 3600,
    },
  });
  expect(
    mintRes.ok(),
    `mint-session failed (${mintRes.status()})`,
  ).toBeTruthy();

  // The response is a whole Set-Cookie header VALUE; we need just the
  // cookie's own value to hand to addCookies.
  const setCookie = ((await mintRes.json()).set_cookie ?? "") as string;
  const value = setCookie.split(";")[0]?.split("=").slice(1).join("=") ?? "";
  expect(value, "mint-session returned no cookie value").not.toBe("");

  // Wait for the merchant's role to be READABLE before handing back a
  // context. onboarding Complete returns before the FGA owner tuple is
  // visible to platform-api, and admin's middleware fails closed on a
  // missing role: it bounces to canonical /login, which then 404s. The
  // spec sees "heading not found" on a page that renders perfectly a
  // second later, so this presented as spec drift in fifteen different
  // places rather than as one race.
  //
  // Polling the exact endpoint the middleware itself calls, rather than
  // sleeping: this is done when the thing that gates rendering says so.
  let role: string | undefined;
  for (let attempt = 0; attempt < 30 && !role; attempt++) {
    const meRes = await request.get(
      `${API_URL}/internal/tenants/${tenant.id}/me?uid=${encodeURIComponent(tenant.owner_user_id)}`,
      { headers: authHeaders },
    );
    if (meRes.ok()) role = ((await meRes.json()).data ?? {}).role;
    if (!role) await new Promise((r) => setTimeout(r, 500));
  }
  expect(
    role,
    `owner role never became visible for tenant ${tenant.id} — admin's ` +
      "middleware would bounce every page to a login that 404s",
  ).toBeTruthy();

  const ctx = await browser.newContext({
    baseURL: `http://${details.slug}-admin.mark8ly.com`,
  });
  await ctx.addCookies([
    {
      name: "m8_session",
      value,
      domain: `${details.slug}-admin.mark8ly.com`,
      path: "/",
    },
  ]);
  return ctx;
}

/**
 * Create products for a freshly-onboarded merchant (#858).
 *
 * Several specs assert against a populated products table but onboard a
 * FRESH tenant, which has none — they were written when the suite mocked
 * its backend and something else supplied the rows. Against a real stack
 * they saw the empty state, which surfaced as "table tbody tr not visible"
 * and read like a rendering bug.
 *
 * Per-spec, not a shared fixture. Shared mutable fixtures across specs are
 * what made the old suite ordering-dependent, and the design for #858
 * called that out explicitly as the thing to avoid.
 *
 * Goes through marketplace-api's real admin API as the real owner. The
 * owner's uid matters: the route authorises against an FGA role, so a
 * synthetic user id gets a 404 that looks like a missing store.
 */
export async function seedProducts(
  request: APIRequestContext,
  details: { email: string },
  count = 3,
): Promise<void> {
  const secret = process.env.INTERNAL_AUTH_SECRET ?? "";
  const authHeaders = { "X-Internal-Auth": secret };

  const tenantRes = await request.get(
    `${API_URL}/internal/tenants/by-owner-email?email=${encodeURIComponent(details.email)}`,
    { headers: authHeaders },
  );
  expect(tenantRes.ok(), "by-owner-email failed while seeding").toBeTruthy();
  const tenant = (await tenantRes.json()).data as {
    id: string;
    owner_user_id: string;
  };

  // marketplace-api keeps its OWN store rows (created by ensure-self-store
  // during onboarding), with different ids from platform-api's. Ask it.
  const scope = {
    "X-Tenant-Id": tenant.id,
    "X-User-Id": tenant.owner_user_id,
  };
  // Poll: onboarding Complete calls ensure-self-store/ensure-self-vendor on
  // marketplace-api, and neither the store row nor the marketplace-store FGA
  // grant is readable the instant Complete returns. Same race as the owner
  // role in signInAsOwner — an immediate call 404s, which reads as "this
  // merchant has no store" rather than "not yet".
  let stores: { id: string }[] = [];
  for (let attempt = 0; attempt < 30 && stores.length === 0; attempt++) {
    const res = await request.get(`${MARKETPLACE_API_URL}/api/v1/admin/stores`, {
      headers: scope,
    });
    if (res.ok()) stores = ((await res.json()).data ?? []) as { id: string }[];
    if (stores.length === 0) await new Promise((r) => setTimeout(r, 500));
  }
  expect(
    stores.length,
    "marketplace-api never reported a store for this merchant — " +
      "ensure-self-store did not land",
  ).toBeGreaterThan(0);
  const storeId = stores[0]!.id;

  for (let i = 1; i <= count; i++) {
    const res = await request.post(
      `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/products`,
      {
        headers: { ...scope, "Content-Type": "application/json" },
        data: {
          title: `Seeded Product ${i}`,
          status: "active",
          variants: [
            {
              sku: `SEED-${i}`,
              price: "10.00",
              inventory_quantity: 5,
            },
          ],
        },
      },
    );
    expect(res.ok(), `seeding product ${i} failed (${res.status()})`).toBeTruthy();
  }
}
