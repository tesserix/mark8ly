"use server";

// Server action for storefront customer sign-in.
//
// Customer auth skips auth-bff's merchant gauntlet entirely — auth-bff's
// auto-login checks OpenFGA tenant membership which customers don't have.
// Instead we:
//   1. Verify the credential — under GIP, the Identity Toolkit id_token
//      (signature, project, tenant, expiry); under Zitadel, the login
//      name + password via auth-bff's storefront-customer endpoint
//      (see @/lib/auth/auth-bff-customer). This is the ONLY step that
//      differs between providers — see the comment on AUTH_PROVIDER below.
//   2. Set an HMAC-signed `mp_customer_session` cookie.
//   3. Ensure the customer profile exists in marketplace-api.

import { cookies, headers } from "next/headers";
import { encodeSession } from "@/lib/session";
import {
  GIPTokenVerificationError,
  verifyGIPIdToken,
} from "@/lib/gip/verify-id-token";
import { verifyCustomerCredential } from "@/lib/auth/auth-bff-customer";
import { sanitizeHost } from "@/lib/host";
import { platformInternalFetch } from "@/lib/api/server/platformInternal";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";
const GIP_PROJECT_ID = process.env.GIP_PROJECT_ID ?? "";
const GIP_CUSTOMER_TENANT_ID = process.env.GIP_CUSTOMER_TENANT_ID ?? "";

// Which identity provider verifies the credential in customerSignIn below.
// Read defensively, matching apps/admin/lib/config.ts's publicConfig.authProvider
// rule exactly: only the literal string "zitadel" switches the path — unset,
// empty, or any other value (including "Zitadel" or "true") stays on GIP.
// NEXT_PUBLIC_-prefixed (not server-only) because CustomerSignInForm, a client
// component, must branch on the identical value to decide whether to call
// Identity Toolkit from the browser at all.
const AUTH_PROVIDER: "gip" | "zitadel" =
  process.env.NEXT_PUBLIC_AUTH_PROVIDER === "zitadel" ? "zitadel" : "gip";

type Result =
  | { ok: true }
  | { ok: false; code: string; message: string };

interface CustomerSignInInput {
  storeSlug: string;
  /** GIP path only: the verified Identity Toolkit id_token. */
  idToken?: string;
  /** Deprecated: ignored. The trusted UID comes from the verified credential. */
  uid?: string;
  /** Deprecated: ignored. The trusted email comes from the verified credential. */
  email?: string;
  /** Zitadel path only: the login name (email) collected by the form. */
  loginName?: string;
  /** Zitadel path only: the password collected by the form. Never logged. */
  password?: string;
  /**
   * Zitadel path only: the auth-bff `auth_request_id` for this login
   * attempt. Obtaining a real one requires an OIDC authorize round-trip
   * (see apps/admin/app/login/authorize/route.ts for the merchant analog)
   * that is out of scope for this task — see the phase's progress ledger.
   * Defaults to "" until that plumbing exists, which auth-bff rejects
   * with a 400 today, so the Zitadel path is not yet reachable end-to-end
   * outside tests.
   */
  authRequestId?: string;
}

async function resolveStore(
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

export async function customerSignIn(
  input: CustomerSignInInput,
): Promise<Result> {
  try {
    // Resolve and validate the customer-facing host first — it gates
    // the cookie Domain, so an invalid host means we cannot complete
    // sign-in regardless of token validity.
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

    const store = await resolveStore(input.storeSlug);
    if (!store) {
      return {
        ok: false,
        code: "store_not_found",
        message: "Could not resolve this store. Please try again.",
      };
    }

    // The ONLY step that differs between providers: how the credential is
    // verified and where {uid, email} come from. Everything below this
    // block — cookie minting, domain scoping, profile/loyalty side
    // effects — is identical on both paths.
    let verified: { uid: string; email: string };
    if (AUTH_PROVIDER === "zitadel") {
      if (!input.loginName || !input.password) {
        return {
          ok: false,
          code: "invalid_request",
          message: "Email and password are required.",
        };
      }
      const outcome = await verifyCustomerCredential({
        authRequestId: input.authRequestId ?? "",
        loginName: input.loginName,
        password: input.password,
      });
      if (outcome.kind !== "complete") {
        // Covers `rejected` (wrong credential — collapsed to one outcome,
        // see auth-bff-customer.ts) as well as `totp_required` and
        // `handoff`, neither of which the storefront form collects yet.
        // No cookie is set in any of these cases.
        return {
          ok: false,
          code: "invalid_credentials",
          message: "Email or password is incorrect.",
        };
      }
      verified = { uid: outcome.uid, email: outcome.email };
    } else {
      verified = await verifyGIPIdToken(
        input.idToken ?? "",
        GIP_PROJECT_ID,
        GIP_CUSTOMER_TENANT_ID,
      );
    }

    // Set the HMAC-signed customer session cookie. The storefront
    // layout decodes and verifies this on every request to hydrate
    // the authenticated state.
    const cookieValue = encodeSession({
      uid: verified.uid,
      email: verified.email,
      store_slug: input.storeSlug,
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
    await ensureCustomerProfile(input.storeSlug, cookieValue);

    // Auto-enroll in loyalty (idempotent — signup bonus awarded once).
    // Reads the mp_referral cookie captured by middleware on a prior
    // page hit, so referral attribution survives the GIP signup dance.
    const referralCode = c.get("mp_referral")?.value;
    await ensureLoyaltyEnrollment(input.storeSlug, verified.email, referralCode);
    if (referralCode) {
      // Burn the cookie so the same invite link can't be replayed by
      // the same browser for another account.
      c.delete("mp_referral");
    }

    return { ok: true };
  } catch (err) {
    if (err instanceof GIPTokenVerificationError) {
      return {
        ok: false,
        code: "invalid_token",
        message: "Your sign-in session could not be verified. Please sign in again.",
      };
    }
    return {
      ok: false,
      code: "unknown",
      message: err instanceof Error ? err.message : String(err),
    };
  }
}
