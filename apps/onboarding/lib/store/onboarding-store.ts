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
