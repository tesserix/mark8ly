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
    <div className="w-full max-w-xl mx-auto overflow-hidden rounded-[2rem] border border-warm-200/90 bg-white/90 shadow-[0_24px_80px_rgba(43,38,34,0.12)] backdrop-blur-sm">
      <div className="border-b border-warm-100 bg-[linear-gradient(180deg,rgba(243,238,230,0.72),rgba(255,255,255,0.98))] px-8 py-8 text-center">
        <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-full border border-sage-200 bg-sage-50 text-3xl shadow-sm" aria-hidden>
          ✉
        </div>
        <p className="mt-5 text-xs font-medium uppercase tracking-[0.16em] text-foreground-tertiary">
          Step 2 of 2
        </p>
        <h1 className="mt-2 font-serif text-3xl font-medium tracking-tight text-foreground">
          Check your inbox
        </h1>
        <p className="mt-3 text-sm leading-6 text-foreground-secondary">
          We sent a verification link to{" "}
          <strong className="font-medium text-foreground">
            {email || "your email"}
          </strong>
          . Open the email on this device to finish launching your store.
        </p>
      </div>

      <div className="px-8 py-8">
        <div className="rounded-2xl border border-warm-200 bg-warm-50/70 p-5">
          <p className="text-sm font-medium text-foreground">
            Keep this tab open while you check your inbox.
          </p>
          <p className="mt-2 text-sm leading-6 text-foreground-secondary">
            The verification link will bring you right back here and complete
            setup automatically.
          </p>
        </div>

        <div className="mt-6 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-sm text-foreground-secondary">
            Didn&apos;t receive it yet?
          </p>
          <button
            onClick={resend}
            disabled={resendIn > 0 || pending || !sessionId}
            className="inline-flex items-center justify-center rounded-full border border-warm-300 bg-white px-5 py-2.5 text-sm font-medium text-foreground transition-[background-color,border-color,color,box-shadow] hover:border-warm-400 hover:bg-warm-50 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {pending
              ? "Sending…"
              : resendIn > 0
                ? `Resend in ${resendIn}s`
                : "Resend link"}
          </button>
        </div>

        {message && (
          <p className="mt-4 text-sm text-foreground-secondary" aria-live="polite">
            {message}
          </p>
        )}
      </div>
    </div>
  );
}
