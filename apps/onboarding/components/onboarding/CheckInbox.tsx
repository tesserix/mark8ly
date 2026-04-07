"use client";

import { useEffect, useState, useTransition } from "react";

import { useOnboardingStore } from "@/lib/store/onboarding-store";
import { resendMagicLink } from "@/app/onboarding/actions";

const RESEND_COOLDOWN_SECONDS = 30;

export function CheckInbox() {
  const email = useOnboardingStore((s) => s.email);
  const sessionId = useOnboardingStore((s) => s.sessionId);
  const businessName = useOnboardingStore((s) => s.businessName);

  const [resendIn, setResendIn] = useState(0);
  const [message, setMessage] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  useEffect(() => {
    if (resendIn <= 0) return;
    const t = setTimeout(() => setResendIn(resendIn - 1), 1000);
    return () => clearTimeout(t);
  }, [resendIn]);

  function resend() {
    if (resendIn > 0 || !sessionId) return;
    setMessage(null);
    setResendIn(RESEND_COOLDOWN_SECONDS);
    startTransition(async () => {
      const r = await resendMagicLink(sessionId, businessName);
      if (!r.ok) {
        setMessage(r.message);
        setResendIn(0);
      } else {
        setMessage("Sent! Check your inbox again.");
      }
    });
  }

  return (
    <div className="w-full max-w-md mx-auto bg-white dark:bg-zinc-900 rounded-2xl shadow-sm border border-zinc-200 dark:border-zinc-800 p-8 text-center">
      <div className="text-5xl mb-4" aria-hidden>📬</div>
      <h1 className="text-2xl font-semibold tracking-tight">Check your inbox</h1>
      <p className="mt-3 text-sm text-zinc-600 dark:text-zinc-400">
        We sent a verification link to{" "}
        <strong className="text-zinc-900 dark:text-white">{email || "your email"}</strong>.
        Click the button in the email to finish setting up your store.
      </p>

      <div className="mt-8 text-sm text-zinc-500">
        Didn't receive it?{" "}
        <button
          onClick={resend}
          disabled={resendIn > 0 || pending || !sessionId}
          className="text-zinc-900 dark:text-white font-medium underline disabled:no-underline disabled:text-zinc-400"
        >
          {resendIn > 0 ? `Resend in ${resendIn}s` : "Resend link"}
        </button>
      </div>

      {message && <p className="mt-3 text-xs text-zinc-500">{message}</p>}
    </div>
  );
}
