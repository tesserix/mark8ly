"use client";

import { useState, useTransition } from "react";

import { useWizardStore } from "@/lib/store/wizard-store";
import { createSession, sendVerification } from "@/app/onboarding/actions";

export function EmailStep() {
  const setEmail = useWizardStore((s) => s.setEmail);
  const setSessionId = useWizardStore((s) => s.setSessionId);
  const setStep = useWizardStore((s) => s.setStep);

  const [email, setLocalEmail] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    const trimmed = email.trim().toLowerCase();
    if (!trimmed.includes("@")) {
      setError("Please enter a valid email address");
      return;
    }

    startTransition(async () => {
      const r1 = await createSession(trimmed);
      if (!r1.ok) {
        setError(r1.message);
        return;
      }
      const sessionId = r1.data.sessionId;
      // Fire-and-await: send the OTP. If it fails the user can resend.
      const r2 = await sendVerification(sessionId);
      if (!r2.ok) {
        setError(r2.message);
        return;
      }
      // Commit to the store + advance.
      setEmail(trimmed);
      setSessionId(sessionId);
      setStep("verify");
    });
  }

  return (
    <form onSubmit={handleSubmit}>
      <h1 className="text-2xl font-semibold tracking-tight mb-2">
        Get started
      </h1>
      <p className="text-sm text-zinc-500 dark:text-zinc-400 mb-6">
        We'll email you a verification code to get your store live in under
        two minutes.
      </p>

      <label
        htmlFor="email"
        className="block text-sm font-medium text-zinc-900 dark:text-zinc-100 mb-1"
      >
        Email address
      </label>
      <input
        id="email"
        type="email"
        value={email}
        onChange={(e) => setLocalEmail(e.target.value)}
        placeholder="founder@yourbusiness.com"
        required
        autoComplete="email"
        autoFocus
        disabled={pending}
        className="w-full px-3 py-2 rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 text-zinc-900 dark:text-zinc-100 placeholder:text-zinc-400 focus:outline-none focus:ring-2 focus:ring-zinc-900 dark:focus:ring-white"
      />

      {error && (
        <p className="mt-2 text-sm text-red-600" role="alert">
          {error}
        </p>
      )}

      <button
        type="submit"
        disabled={pending}
        className="mt-6 w-full bg-zinc-900 hover:bg-zinc-800 disabled:bg-zinc-400 dark:bg-white dark:hover:bg-zinc-100 text-white dark:text-zinc-900 font-medium py-2.5 rounded-lg transition-colors"
      >
        {pending ? "Sending code…" : "Continue"}
      </button>
    </form>
  );
}
