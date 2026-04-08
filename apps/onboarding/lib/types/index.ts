// Hand-written types matching platform-api's domain models.
//
// These will eventually be generated from the Go structs (oapi-codegen or
// similar). For Phase F we hand-write them — there's only one consumer
// today, drift is low-risk, and codegen is its own setup project that
// can land later.

export interface Country {
  code: string;
  name: string;
  native_name: string;
  calling_code: string;
  currency_code: string;
  flag_emoji: string;
  region: string;
}

export interface State {
  id: string;
  country_code: string;
  code: string;
  name: string;
  type: string;
}

export interface Currency {
  code: string;
  name: string;
  symbol: string;
  decimal_places: number;
}

export interface Timezone {
  id: string;
  name: string;
  utc_offset: string;
}

export interface OnboardingDraft {
  business_name?: string;
  slug?: string;
  country_code?: string;
  currency_code?: string;
  timezone?: string;
}

export interface OnboardingSession {
  id: string;
  email: string;
  draft: OnboardingDraft;
  status: "in_progress" | "verifying" | "completed" | "expired" | "abandoned";
  email_verified_at: string | null;
  tenant_id: string | null;
  completed_at: string | null;
  last_activity_at: string;
  created_at: string;
  updated_at: string;
}

export interface CompleteResult {
  tenant_id: string;
  slug: string;
}

// Standard response envelope used by every platform-api endpoint.
export interface DataEnvelope<T> {
  data: T;
}

export interface ErrorEnvelope {
  error: string;
  message: string;
}
