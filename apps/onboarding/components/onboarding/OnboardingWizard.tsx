"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

import type { Country, Currency, Timezone } from "@/lib/types";
import { useWizardStore } from "@/lib/store/wizard-store";
import { ProgressBar } from "./ProgressBar";
import { EmailStep } from "./EmailStep";
import { VerifyStep } from "./VerifyStep";
import { BusinessStep } from "./BusinessStep";
import { SubmittingStep } from "./SubmittingStep";

interface Props {
  countries: Country[];
  currencies: Currency[];
  timezones: Timezone[];
}

/**
 * OnboardingWizard is the single client component that owns step state.
 * Each step is a presentational subcomponent. The store survives reload
 * via sessionStorage so refreshes mid-wizard don't lose progress.
 */
export function OnboardingWizard({ countries, currencies, timezones }: Props) {
  const step = useWizardStore((s) => s.step);
  const router = useRouter();

  // When the wizard reaches "done", redirect to /welcome. The session
  // cookie was already set by completeAndLogin, so /welcome will see
  // an authenticated user.
  useEffect(() => {
    if (step === "done") {
      router.push("/welcome");
    }
  }, [step, router]);

  return (
    <div className="w-full max-w-md mx-auto">
      <ProgressBar current={step} />
      <div className="mt-8 bg-white dark:bg-zinc-900 rounded-2xl shadow-sm border border-zinc-200 dark:border-zinc-800 p-8">
        {step === "email" && <EmailStep />}
        {step === "verify" && <VerifyStep />}
        {step === "business" && (
          <BusinessStep
            countries={countries}
            currencies={currencies}
            timezones={timezones}
          />
        )}
        {step === "submitting" && <SubmittingStep />}
        {step === "done" && (
          <p className="text-center text-zinc-500">Redirecting…</p>
        )}
      </div>
    </div>
  );
}
