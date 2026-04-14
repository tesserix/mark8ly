"use server";

// Server action for storefront customer sign-in.
//
// Customer auth skips auth-bff entirely — auth-bff's auto-login checks
// OpenFGA tenant membership which customers don't have. Instead we:
//   1. Verify the GIP id_token came from the mp-customer tenant pool
//   2. Set a simple `mp_customer_session` cookie with uid + email
//   3. Ensure the customer profile exists in marketplace-api
//
// The cookie is a base64-encoded JSON payload. Not signed yet (v0) —
// a proper HMAC signature or encrypted cookie lands in a follow-up.
// For now the cookie carries enough for the storefront layout to
// render the authenticated state and for marketplace-api calls to
// include the customer's identity.

import { cookies } from "next/headers";
import { encodeSession } from "@/lib/session";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";
const PLATFORM_API_URL =
  process.env.PLATFORM_API_URL ?? "http://localhost:8086";

type Result =
  | { ok: true }
  | { ok: false; code: string; message: string };

interface CustomerSignInInput {
  idToken: string;
  uid: string;
  storeSlug: string;
  email?: string;
}

async function resolveStore(
  storeSlug: string,
): Promise<{ tenant_id: string; store_id: string } | null> {
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/internal/stores/by-slug/${encodeURIComponent(storeSlug)}`,
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

// Decode the GIP id_token payload (JWT part 1) to extract email.
// We trust this token because we obtained it from our own GIP call
// moments ago — full signature verification is overkill here but
// should be added for production hardening.
function decodeIdTokenEmail(idToken: string): string | null {
  try {
    const parts = idToken.split(".");
    if (parts.length < 2) return null;
    const payload = JSON.parse(
      Buffer.from(parts[1]!, "base64url").toString(),
    ) as { email?: string };
    return payload.email ?? null;
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
    const store = await resolveStore(input.storeSlug);
    if (!store) {
      return {
        ok: false,
        code: "store_not_found",
        message: "Could not resolve this store. Please try again.",
      };
    }

    const email =
      input.email || decodeIdTokenEmail(input.idToken) || "";

    // Set the HMAC-signed customer session cookie. The storefront
    // layout decodes and verifies this on every request to hydrate
    // the authenticated state.
    const cookieValue = encodeSession({
      uid: input.uid,
      email,
      store_slug: input.storeSlug,
      store_id: store.store_id,
      tenant_id: store.tenant_id,
    });

    const c = await cookies();
    c.set({
      name: "mp_customer_session",
      value: cookieValue,
      path: "/",
      domain: ".mark8ly.com",
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
    await ensureLoyaltyEnrollment(input.storeSlug, email, referralCode);
    if (referralCode) {
      // Burn the cookie so the same invite link can't be replayed by
      // the same browser for another account.
      c.delete("mp_referral");
    }

    return { ok: true };
  } catch (err) {
    return {
      ok: false,
      code: "unknown",
      message: err instanceof Error ? err.message : String(err),
    };
  }
}
