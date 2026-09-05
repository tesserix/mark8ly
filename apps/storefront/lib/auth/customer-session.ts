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
// issueCustomerSession stays the ONLY place that mints
// `mp_customer_session`, and only completeCustomerSignIn and
// completeCustomerStoreJoin (both below) call it —
// apps/storefront/app/sign-in/actions.ts's customerSignIn and
// confirmCustomerTotp, app/create-account/actions.ts's
// verifyCustomerEmail, app/join/actions.ts's joinThisStore, and
// app/auth/idp/finish/route.ts (the Zitadel Google finish route) all go
// through those two rather than each minting their own cookie. A second
// cookie-minting path would drift from this one the same way a second
// copy of this function's logic would.

import { cookies } from "next/headers";
import { encodeSession, SESSION_TTL_SECONDS } from "@/lib/session";
import {
  JOIN_GRANT_COOKIE,
  JOIN_GRANT_TTL_SECONDS,
  signJoinGrant,
} from "@/lib/auth/join-grant";
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

// Asks marketplace-api whether this verified identity has already joined
// this store. The freshly minted cookie value is sent as a header on a
// server-to-server call — it is NOT handed to the browser unless this
// comes back a member, which is the entire point: a session must never be
// minted at a store the customer has not joined.
//
// This replaced a "best-effort" GET /account whose only purpose was the
// side effect on the other end: marketplace-api's session middleware used
// to CREATE the customer_profiles row when it saw a valid cookie. That is
// exactly the bug in
// docs/superpowers/specs/2026-09-05-customer-store-membership-design.md —
// browsing store2 with a store1 password made you a customer of store2.
// The middleware is read-only now, and creation lives behind the explicit
// join below.
//
// Fails CLOSED: an unreachable or unparseable API means we cannot show
// that the customer belongs here, so we do not mint a session. The caller
// turns that into a plain "try again shortly", never a silent sign-in.
type MembershipCheck =
  | { kind: "member" }
  | { kind: "not_member" }
  | { kind: "unavailable" };

async function checkStoreMembership(
  storeSlug: string,
  cookieValue: string,
): Promise<MembershipCheck> {
  try {
    const headers: Record<string, string> = {
      Accept: "application/json",
      Cookie: `mp_customer_session=${cookieValue}`,
    };
    if (STOREFRONT_KEY) headers["X-Storefront-Key"] = STOREFRONT_KEY;

    const res = await fetch(
      `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(storeSlug)}/account/membership`,
      { method: "GET", headers, cache: "no-store" },
    );
    if (!res.ok) return { kind: "unavailable" };
    const body = (await res.json()) as { data?: { member?: boolean } };
    if (typeof body.data?.member !== "boolean") return { kind: "unavailable" };
    return body.data.member ? { kind: "member" } : { kind: "not_member" };
  } catch {
    return { kind: "unavailable" };
  }
}

/**
 * joinStoreWithSession creates the customer_profiles membership row for
 * an identity this server has verified. It is the ONLY thing in this app
 * that asks marketplace-api to create a membership, and it runs only
 * after the customer explicitly accepted the join.
 *
 * Authenticates with the same signed session value the join is about, so
 * marketplace-api creates the membership for the identity IN that
 * credential — never for an email a caller supplied.
 */
async function joinStoreWithSession(
  storeSlug: string,
  cookieValue: string,
): Promise<{ ok: true } | { ok: false; code: string; message: string }> {
  let res: Response;
  try {
    const headers: Record<string, string> = {
      Accept: "application/json",
      "Content-Type": "application/json",
      Cookie: `mp_customer_session=${cookieValue}`,
    };
    if (STOREFRONT_KEY) headers["X-Storefront-Key"] = STOREFRONT_KEY;

    res = await fetch(
      `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(storeSlug)}/account/join`,
      { method: "POST", headers, body: "{}", cache: "no-store" },
    );
  } catch {
    return {
      ok: false,
      code: "join_unavailable",
      message: "We couldn't finish setting up your account here. Please try again shortly.",
    };
  }

  if (res.ok) return { ok: true };

  const body = (await res.json().catch(() => null)) as {
    error?: string;
  } | null;
  if (body?.error === "account_blocked") {
    return {
      ok: false,
      code: "account_blocked",
      message:
        "This store has suspended your account, so it can't be reopened here. Please contact the store if you think that's a mistake.",
    };
  }
  return {
    ok: false,
    code: "join_failed",
    message: "We couldn't finish setting up your account here. Please try again shortly.",
  };
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
 * verified {uid, email}: apps/storefront/app/sign-in/actions.ts's
 * customerSignIn (password/id-token) and confirmCustomerTotp
 * (authenticator code), apps/storefront/app/create-account/actions.ts's
 * verifyCustomerEmail, and apps/storefront/app/auth/idp/finish/route.ts
 * (the Zitadel Google finish route) all call this instead of each
 * carrying their own copy.
 *
 * A verified credential is NOT on its own permission to be here. One
 * Mark8ly login works platform-wide; each store is a membership the
 * customer joins deliberately. So this function checks membership BEFORE
 * the cookie reaches the browser:
 *
 *   member      -> mint the session, run the loyalty/referral side effects
 *   not a member-> mint NOTHING; issue a short-lived join grant cookie and
 *                  return `membership_required` so the caller can offer
 *                  the join. The copy must never imply a wrong password —
 *                  the password was right.
 *   unavailable -> mint NOTHING. Fail closed and say so.
 */
export async function completeCustomerSignIn(
  store: { tenant_id: string; store_id: string },
  cookieHost: string,
  storeSlug: string,
  verified: { uid: string; email: string },
): Promise<Result> {
  // Minted, but deliberately not handed to the browser yet — it is first
  // used as the server-to-server credential for the membership check.
  const cookieValue = encodeSession({
    uid: verified.uid,
    email: verified.email,
    store_slug: storeSlug,
    store_id: store.store_id,
    tenant_id: store.tenant_id,
  });

  const membership = await checkStoreMembership(storeSlug, cookieValue);
  const c = await cookies();

  if (membership.kind === "unavailable") {
    return {
      ok: false,
      code: "membership_unavailable",
      message: "Sign-in is temporarily unavailable. Please try again shortly.",
    };
  }

  if (membership.kind === "not_member") {
    // No session cookie. The grant below authorises the join and nothing
    // else, and expires in minutes.
    c.set({
      name: JOIN_GRANT_COOKIE,
      value: signJoinGrant({
        uid: verified.uid,
        email: verified.email,
        store_slug: storeSlug,
        store_id: store.store_id,
        tenant_id: store.tenant_id,
      }),
      path: "/",
      domain: cookieHost,
      httpOnly: true,
      secure: true,
      sameSite: "lax",
      maxAge: JOIN_GRANT_TTL_SECONDS,
    });
    return {
      ok: false,
      code: "membership_required",
      message:
        "Your Mark8ly login works here — you just don't have an account with this store yet.",
    };
  }

  issueCustomerSession(c, cookieValue, cookieHost);
  await runPostSignInSideEffects(c, storeSlug, verified.email);
  return { ok: true };
}

/**
 * completeCustomerStoreJoin creates the membership for an identity this
 * server verified minutes ago (carried by a join grant), then completes
 * sign-in exactly as completeCustomerSignIn's member branch does.
 *
 * The order matters and is the invariant this whole feature rests on: the
 * membership row is created FIRST, by an explicit request the customer
 * made, and only then does a session cookie exist.
 */
export async function completeCustomerStoreJoin(
  grant: {
    uid: string;
    email: string;
    store_slug: string;
    store_id: string;
    tenant_id: string;
  },
  cookieHost: string,
): Promise<Result> {
  const cookieValue = encodeSession({
    uid: grant.uid,
    email: grant.email,
    store_slug: grant.store_slug,
    store_id: grant.store_id,
    tenant_id: grant.tenant_id,
  });

  const joined = await joinStoreWithSession(grant.store_slug, cookieValue);
  if (!joined.ok) {
    return { ok: false, code: joined.code, message: joined.message };
  }

  const c = await cookies();
  issueCustomerSession(c, cookieValue, cookieHost);
  // The grant has done its job; leaving it set would keep a second
  // credential alive for no reason.
  c.delete(JOIN_GRANT_COOKIE);
  await runPostSignInSideEffects(c, grant.store_slug, grant.email);
  return { ok: true };
}

type CookieJar = Awaited<ReturnType<typeof cookies>>;

/** The single place `mp_customer_session` is handed to a browser. */
function issueCustomerSession(
  c: CookieJar,
  cookieValue: string,
  cookieHost: string,
): void {
  c.set({
    name: "mp_customer_session",
    value: cookieValue,
    path: "/",
    domain: cookieHost, // scoped to exact host so store-a's session can't be sent to store-b
    httpOnly: true,
    secure: true,
    sameSite: "lax",
    maxAge: SESSION_TTL_SECONDS,
  });
}

/** Loyalty auto-enrolment and referral burn — run once a session exists
 *  and the customer is a member of this store. */
async function runPostSignInSideEffects(
  c: CookieJar,
  storeSlug: string,
  email: string,
): Promise<void> {
  const referralCode = c.get("mp_referral")?.value;
  await ensureLoyaltyEnrollment(storeSlug, email, referralCode);
  if (referralCode) {
    // Burn the cookie so the same invite link can't be replayed by
    // the same browser for another account.
    c.delete("mp_referral");
  }
}
