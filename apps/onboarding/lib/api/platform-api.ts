// Server-side typed client for platform-api.
//
// Used from server actions and server components only — never imported by
// client components. The base URL comes from PLATFORM_API_URL which is
// not exposed to the browser.
//
// Returns clean unwrapped values; throws on error. Server actions wrap
// the throw in a try/catch and return a discriminated union to the client.

import { config } from "@/lib/config";
import type {
  Country,
  Currency,
  Timezone,
  OnboardingSession,
  CompleteResult,
} from "@/lib/types";

const base = config.platformApiUrl;

class PlatformApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${base}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
    // Server actions cache by default — disable for mutations and live reads.
    cache: "no-store",
  });

  if (!res.ok) {
    let body: { error?: string; message?: string } = {};
    try {
      body = await res.json();
    } catch {
      // ignore parse failures
    }
    throw new PlatformApiError(
      res.status,
      body.error ?? "platform_api_error",
      body.message ?? `HTTP ${res.status}`,
    );
  }

  const body = (await res.json()) as { data: T };
  return body.data;
}

// ─── Locations ──────────────────────────────────────────────────────────
export const locations = {
  listCountries: () => request<Country[]>("/api/v1/locations/countries"),
  listCurrencies: () => request<Currency[]>("/api/v1/locations/currencies"),
  listTimezones: () => request<Timezone[]>("/api/v1/locations/timezones"),
};

// ─── Onboarding sessions ────────────────────────────────────────────────
export const onboarding = {
  createSession: (email: string) =>
    request<OnboardingSession>("/api/v1/onboarding/sessions", {
      method: "POST",
      body: JSON.stringify({ email }),
    }),

  getSession: (sessionId: string) =>
    request<OnboardingSession>(`/api/v1/onboarding/sessions/${sessionId}`),

  saveDraft: (sessionId: string, draft: Record<string, unknown>) =>
    request<{ saved: boolean }>(
      `/api/v1/onboarding/sessions/${sessionId}/draft`,
      { method: "PATCH", body: JSON.stringify(draft) },
    ),

  sendVerification: (sessionId: string, businessName?: string) =>
    request<{ sent: boolean }>(
      `/api/v1/onboarding/sessions/${sessionId}/verification/send`,
      {
        method: "POST",
        body: JSON.stringify({ business_name: businessName ?? "" }),
      },
    ),

  /** Asks what a promo code would grant, WITHOUT redeeming it (#620).
   *
   *  Goes through platform-api rather than straight to marketplace-api:
   *  marketplace-api's validate route is guarded by the shared internal
   *  secret, which platform-api holds and this app does not. That is the
   *  point — #620 calls an open validate endpoint "an oracle for guessing
   *  valid codes", and routing it this way means no such endpoint exists. */
  validatePromo: (code: string, email: string, currency: string) =>
    request<{
      valid: boolean;
      trial_extension_days: number;
      reject_reason: string;
    }>("/api/v1/onboarding/promo/validate", {
      method: "POST",
      body: JSON.stringify({ code, email, currency }),
    }),

  verifyCode: (sessionId: string, code: string) =>
    request<{ verified: boolean }>(
      `/api/v1/onboarding/sessions/${sessionId}/verification/verify`,
      { method: "POST", body: JSON.stringify({ code }) },
    ),

  complete: (
    sessionId: string,
    body: {
      business_name: string;
      slug: string;
      /** GIP path only. On the Zitadel path the merchant has no provider
       *  account yet — platform-api's complete endpoint creates it — so
       *  there is no id to send and the field is omitted entirely. */
      owner_user_id?: string;
      owner_email: string;
      country_code: string;
      currency_code: string;
      timezone: string;
      /** Zitadel path only: the password platform-api creates the
       *  merchant's Zitadel account with. Never sent under GIP, where the
       *  browser already created the account. */
      password?: string;
      first_name?: string;
      last_name?: string;
      /** What the merchant typed in the promo field, if anything (#620).
       *  Redeemed by marketplace-api immediately after it creates the
       *  subscription row — the earliest moment redemption is possible,
       *  since the ledger row needs a subscription id. */
      promo_code?: string;
      /** The Tax ID the merchant typed on the form, unvalidated.
       *
       *  Sent because signup is the only point it is known. It was written
       *  to the session draft from §5.1.1 onward and read by nobody:
       *  platform-api had no `tax_id` at all, and marketplace-api's
       *  reverse_charge_tax_id column had no writer anywhere in the tree
       *  while two consumers read it. */
      tax_id?: string;
    },
  ) =>
    request<CompleteResult>(
      `/api/v1/onboarding/sessions/${sessionId}/complete`,
      { method: "POST", body: JSON.stringify(body) },
    ),
};

// ─── Tenants ────────────────────────────────────────────────────────────
//
// Phase Q: slug availability moved to the store package — a slug is
// a store-level identifier now. The onboarding wizard still calls
// this during the wizard's draft phase, before the store row
// actually exists, so "available" means "no other store anywhere
// has claimed this slug yet".
export const tenants = {
  isSlugAvailable: (slug: string) =>
    request<{ slug: string; available: boolean }>(
      `/api/v1/stores/slug-available?slug=${encodeURIComponent(slug)}`,
    ),
};

export interface Membership {
  tenant_id: string;
  name: string;
  role: string;
}

export const users = {
  listMemberTenants: (uid: string) =>
    request<Membership[]>(
      `/api/v1/users/me/tenants?uid=${encodeURIComponent(uid)}`,
    ),
};

export { PlatformApiError };
