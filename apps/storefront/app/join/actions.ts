"use server";

// The explicit store join.
//
// A Mark8ly login is one platform identity; each store is a separate
// membership. Signing in at a store the customer has not joined mints no
// session — it leaves a short-lived join grant and sends them here (see
// @/lib/auth/join-grant and
// docs/superpowers/specs/2026-09-05-customer-store-membership-design.md).
//
// This action is what turns "I have a login" into "I have an account with
// this store": it creates the customer_profiles row and nothing else — no
// new identity, no new credential, no change to any other store the
// customer belongs to.
//
// Every export is async, because Next.js silently strips non-async
// runtime exports from a `"use server"` module (see
// @/lib/auth/customer-sign-in-result's header for the time that bit this
// repo). The join-grant claim shape and its verifier are plain functions
// and live in @/lib/auth/join-grant accordingly.

import { cookies, headers } from "next/headers";
import { resolveStoreSlug } from "@/lib/slug";
import { sanitizeHost } from "@/lib/host";
import { JOIN_GRANT_COOKIE, verifyJoinGrant } from "@/lib/auth/join-grant";
import { completeCustomerStoreJoin } from "@/lib/auth/customer-session";
import type { CustomerSignInResult as Result } from "@/lib/auth/customer-sign-in-result";

const EXPIRED_MESSAGE =
  "This join request has expired. Please sign in again to join this store.";

/**
 * joinThisStore creates the membership for the identity carried by the
 * join-grant cookie, then completes sign-in.
 *
 * It takes NO caller-supplied identity: the email and uid come from the
 * grant this server signed after verifying the credential itself, so a
 * caller cannot join a store as someone else. The grant is also checked
 * against the store the request actually arrived at, so a grant issued at
 * store2 cannot be spent at store3.
 */
export async function joinThisStore(): Promise<Result> {
  const h = await headers();
  const rawHost = h.get("x-forwarded-host") ?? h.get("host");
  const cookieHost = sanitizeHost(rawHost);
  if (!cookieHost) {
    return {
      ok: false,
      code: "invalid_host",
      message: "Could not validate the host for sign-in. Please try again.",
    };
  }

  const storeSlug = await resolveStoreSlug(rawHost);
  if (!storeSlug) {
    return {
      ok: false,
      code: "store_not_found",
      message: "Could not resolve this store. Please try again.",
    };
  }

  const c = await cookies();
  const grant = verifyJoinGrant(c.get(JOIN_GRANT_COOKIE)?.value, storeSlug);
  if (!grant) {
    // Expired, tampered with, or minted for a different store. All three
    // recover the same way, and none of them is a password problem.
    return { ok: false, code: "join_grant_invalid", message: EXPIRED_MESSAGE };
  }

  return completeCustomerStoreJoin(grant, cookieHost);
}

/**
 * pendingJoinEmail reports the address the join screen is about, so the
 * page can name it. Returns null when there is no usable grant, which the
 * page renders as "start again" rather than an empty join form.
 */
export async function pendingJoinEmail(
  storeSlug: string,
): Promise<string | null> {
  const c = await cookies();
  const grant = verifyJoinGrant(c.get(JOIN_GRANT_COOKIE)?.value, storeSlug);
  return grant?.email ?? null;
}
