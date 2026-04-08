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
      owner_user_id: string;
      owner_email: string;
      country_code: string;
      currency_code: string;
      timezone: string;
    },
  ) =>
    request<CompleteResult>(
      `/api/v1/onboarding/sessions/${sessionId}/complete`,
      { method: "POST", body: JSON.stringify(body) },
    ),
};

// ─── Tenants ────────────────────────────────────────────────────────────
interface TenantSummary {
  id: string;
  slug: string;
  name: string;
  owner_user_id: string;
  owner_email: string;
}

export const tenants = {
  isSlugAvailable: (slug: string) =>
    request<{ slug: string; available: boolean }>(
      `/api/v1/tenants/slug-available?slug=${encodeURIComponent(slug)}`,
    ),

  /** Look up the workspace tenant owned by the given GIP UID. Used by
   *  the returning-user sign-in path so the server action can fill the
   *  workspace_tenant field in the auth-bff /auth/auto-login call. */
  getByOwner: (uid: string) =>
    request<TenantSummary>(
      `/api/v1/tenants/by-owner?uid=${encodeURIComponent(uid)}`,
    ),
};

export { PlatformApiError };
