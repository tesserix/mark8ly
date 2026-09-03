// resolveStore and completeCustomerSignIn — the store lookup and the
// single tail that mints `mp_customer_session` — used to live in
// apps/storefront/app/sign-in/actions.ts, a `"use server"` module.
//
// They live here instead, in a plain module, because neither is a
// server action: only customerSignIn and confirmCustomerTotp (still in
// actions.ts) are ever called from a client component. Next.js has not
// registered completeCustomerSignIn/resolveStore in the server-actions
// manifest today because nothing imports them from client code, but
// that is a property of the current call graph, not of the module they
// sit in — the moment any client component imports
// completeCustomerSignIn from a `"use server"` file, it becomes an
// unauthenticated, publicly callable endpoint that mints a session
// cookie. Moving them to lib/** removes that risk entirely and puts
// them in apps/storefront's vitest-covered path (components/** is not
// covered — see @/lib/auth/provider's file header).
//
// completeCustomerSignIn stays the ONLY place that mints
// `mp_customer_session` — apps/storefront/app/sign-in/actions.ts's
// customerSignIn and confirmCustomerTotp, and
// apps/storefront/app/auth/idp/finish/route.ts (the Zitadel Google
// finish route), all call this same function rather than each minting
// their own cookie. A second cookie-minting path would drift from this
// one the same way a second copy of this function's logic would.

import { cookies } from "next/headers";
import { encodeSession } from "@/lib/session";
import { platformInternalFetch } from "@/lib/api/server/platformInternal";
import type { CustomerSignInResult as Result } from "@/lib/auth/customer-sign-in-result";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";

/**
 * resolveStore looks up a store's tenant_id/store_id by slug via
 * platform-api. Used by both apps/storefront/app/sign-in/actions.ts and
 * apps/storefront/app/auth/idp/finish/route.ts so they share one copy
 * of this lookup rather than each carrying its own.
 */
export async function resolveStore(
  storeSlug: string,
): Promise<{ tenant_id: string; store_id: string } | null> {
  try {
    const res = await platformInternalFetch(
      `/internal/stores/by-slug/${encodeURIComponent(storeSlug)}`,
      { cache: "no-store" },
    );
    if (!res.ok) return null;
    const body = (await res.json()) as {
      data?: { tenant_id?: string; id?: string };
    };
    if (!body.data?.tenant_id || !body.data?.id) return null;
    return { tenant_id: body.data.tenant_id, store_id: body.data.id };
  } catch {
    return null;
  }
}

// Best-effort: register the customer profile in marketplace-api so
// they show up in the merchant's customer list and can place orders.
// We call the web storefront /account endpoint (GET) with the session
// cookie — the OptionalCustomerAuth middleware automatically calls
// EnsureProfile when it sees a valid session cookie.
async function ensureCustomerProfile(
  storeSlug: string,
  cookieValue: string,
): Promise<void> {
  try {
    const headers: Record<string, string> = {
      Accept: "application/json",
      Cookie: `mp_customer_session=${cookieValue}`,
    };
    if (STOREFRONT_KEY) headers["X-Storefront-Key"] = STOREFRONT_KEY;

    await fetch(
      `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(storeSlug)}/account`,
      {
        method: "GET",
        headers,
        cache: "no-store",
      },
    );
    // The GET /account call triggers OptionalCustomerAuth middleware
    // which calls EnsureProfile. We don't care about the response —
    // the side effect (profile creation) is what matters.
  } catch {
    // silent — customer can still browse
  }
}

// Best-effort: auto-enroll the customer in the store's loyalty program.
// Backend is idempotent — returns the existing record on repeat calls.
// If a referral code was captured in the mp_referral cookie, pass it
// through so the referrer and referee bonuses are awarded atomically.
// Silent on failure — loyalty is a growth feature, not a blocker.
async function ensureLoyaltyEnrollment(
  storeSlug: string,
  email: string,
  referralCode: string | undefined,
): Promise<void> {
  if (!email) return;
  try {
    const headers: Record<string, string> = {
      Accept: "application/json",
      "Content-Type": "application/json",
    };
    if (STOREFRONT_KEY) headers["X-Storefront-Key"] = STOREFRONT_KEY;

    const body: Record<string, unknown> = { email };
    if (referralCode) body.referral_code = referralCode;

    await fetch(
      `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(storeSlug)}/loyalty/enroll`,
      {
        method: "POST",
        headers,
        body: JSON.stringify(body),
        cache: "no-store",
      },
    );
  } catch {
    // silent — program may be inactive or network blip
  }
}

/**
 * completeCustomerSignIn is the tail shared by every path that ends in a
 * verified {uid, email}: minting the HMAC-signed session cookie scoped to
 * the resolved host, the best-effort profile and loyalty side effects, and
 * burning the referral cookie. apps/storefront/app/sign-in/actions.ts's
 * customerSignIn (password/id-token path) and confirmCustomerTotp
 * (authenticator-code path), and apps/storefront/app/auth/idp/finish/
 * route.ts (the Zitadel Google finish route, which resolves a
 * {uid, email} from auth-bff's idp/finish endpoint rather than from a
 * password/id-token) all call this instead of each carrying their own
 * copy — a second copy of this tail would drift the moment one path
 * changed and the other didn't.
 */
export async function completeCustomerSignIn(
  store: { tenant_id: string; store_id: string },
  cookieHost: string,
  storeSlug: string,
  verified: { uid: string; email: string },
): Promise<Result> {
  // Set the HMAC-signed customer session cookie. The storefront
  // layout decodes and verifies this on every request to hydrate
  // the authenticated state.
  const cookieValue = encodeSession({
    uid: verified.uid,
    email: verified.email,
    store_slug: storeSlug,
    store_id: store.store_id,
    tenant_id: store.tenant_id,
  });

  const c = await cookies();
  c.set({
    name: "mp_customer_session",
    value: cookieValue,
    path: "/",
    domain: cookieHost, // scoped to exact host so store-a's session can't be sent to store-b
    httpOnly: true,
    secure: true,
    sameSite: "lax",
    maxAge: 60 * 60 * 24 * 30, // 30 days
  });

  // Best-effort profile registration — pass the freshly minted cookie
  // so marketplace-api's OptionalCustomerAuth can validate it and call
  // EnsureProfile.
  await ensureCustomerProfile(storeSlug, cookieValue);

  // Auto-enroll in loyalty (idempotent — signup bonus awarded once).
  // Reads the mp_referral cookie captured by middleware on a prior
  // page hit, so referral attribution survives the GIP signup dance.
  const referralCode = c.get("mp_referral")?.value;
  await ensureLoyaltyEnrollment(storeSlug, verified.email, referralCode);
  if (referralCode) {
    // Burn the cookie so the same invite link can't be replayed by
    // the same browser for another account.
    c.delete("mp_referral");
  }

  return { ok: true };
}
