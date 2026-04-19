"use client";

// Onboarding form store. Holds the form fields between submit and the
// inbox confirmation page so the resend button has context.
//
// Persisted to sessionStorage so a refresh in the middle of the flow
// doesn't blow away progress.

import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";

export interface OnboardingFields {
  email: string;
  sessionId: string;
  businessName: string;
  slug: string;
  countryCode: string;
  currencyCode: string;
  timezone: string;
  // §5.1.1 — tax ID + migration fast-path evidence (optional)
  taxId: string;
  migrationType: "new" | "migrating";
  whoisUrl: string;
  screenshotUrl: string;
}

interface OnboardingState extends OnboardingFields {
  setSubmitted: (input: OnboardingFields) => void;
  reset: () => void;
}

const initial: OnboardingFields = {
  email: "",
  sessionId: "",
  businessName: "",
  slug: "",
  countryCode: "",
  currencyCode: "",
  timezone: "",
  taxId: "",
  migrationType: "new",
  whoisUrl: "",
  screenshotUrl: "",
};

export const useOnboardingStore = create<OnboardingState>()(
  persist(
    (set) => ({
      ...initial,
      setSubmitted: (input) => set(input),
      reset: () => set(initial),
    }),
    {
      name: "m8-onboarding",
      storage: createJSONStorage(() => sessionStorage),
    },
  ),
);
