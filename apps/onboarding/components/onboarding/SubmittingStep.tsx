"use client";

import { useEffect, useState } from "react";

import { useWizardStore } from "@/lib/store/wizard-store";
import { completeAndLogin } from "@/app/onboarding/actions";

/**
 * SubmittingStep is the loading screen shown while we:
 *   1. POST /onboarding/sessions/:id/complete  (creates tenant + outbox FGA writes)
 *   2. POST /auth/auto-login                    (verifies token, retries FGA, mints cookie)
 *
 * Step 2 may take up to ~2 seconds while the FGA outbox drainer ships
 * the membership tuple. This is the bug-fix race window — we hide it
 * behind the loading state instead of surfacing it to the user.
 */
export function SubmittingStep() {
  const sessionId = useWizardStore((s) => s.sessionId);
  const businessName = useWizardStore((s) => s.businessName);
  const slug = useWizardStore((s) => s.slug);
  const email = useWizardStore((s) => s.email);
  const countryCode = useWizardStore((s) => s.countryCode);
  const currencyCode = useWizardStore((s) => s.currencyCode);
  const timezone = useWizardStore((s) => s.timezone);
  const gipUid = useWizardStore((s) => s.gipUid);
  const gipIdToken = useWizardStore((s) => s.gipIdToken);
  const setTenantId = useWizardStore((s) => s.setTenantId);
  const setStep = useWizardStore((s) => s.setStep);

  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const r = await completeAndLogin({
        sessionId,
        businessName,
        slug,
        ownerUserId: gipUid,
        ownerEmail: email,
        countryCode,
        currencyCode,
        timezone,
        idToken: gipIdToken,
      });
      if (cancelled) return;

      if (!r.ok) {
        setError(`${r.code}: ${r.message}`);
        return;
      }
      setTenantId(r.data.tenantId);
      setStep("done");
    })();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="text-center py-8">
      <div className="inline-block w-12 h-12 border-4 border-zinc-200 border-t-zinc-900 dark:border-zinc-800 dark:border-t-white rounded-full animate-spin" />
      <h2 className="mt-6 text-lg font-medium">
        {error ? "Something went wrong" : "Setting up your store…"}
      </h2>
      <p className="mt-2 text-sm text-zinc-500">
        {error
          ? error
          : "Creating your account and store. This usually takes a couple of seconds."}
      </p>
      {error && (
        <button
          onClick={() => setStep("business")}
          className="mt-6 text-sm font-medium underline text-zinc-900 dark:text-white"
        >
          Go back
        </button>
      )}
    </div>
  );
}
