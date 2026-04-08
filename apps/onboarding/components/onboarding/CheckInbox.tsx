"use client";

import { useEffect, useState, useTransition } from "react";

import { useOnboardingStore } from "@/lib/store/onboarding-store";
import { resendMagicLink } from "@/app/onboarding/actions";

const RESEND_COOLDOWN_SECONDS = 30;

/**
 * CheckInbox — rendered inside PostSubmitShell on /onboarding/check-inbox.
 *
 * The shell already provides the eyebrow, title, and lede. This component
 * only renders what comes after: the email the link was sent to, the
 * resend control, and two quiet tips. No inner card chrome, no duplicate
 * h1, no decorative gradient header.
 */
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
        setMessage("Sent — check your inbox again.");
      }
    });
  }

  return (
    <div className="border-t border-border-subtle pt-10">
      <dl className="grid gap-2">
        <dt className="eyebrow">Link sent to</dt>
        <dd className="font-serif text-xl text-foreground">
          {email || "your email"}
        </dd>
      </dl>

      <div className="mt-10 flex flex-wrap items-center gap-x-8 gap-y-4">
        <button
          type="button"
          onClick={resend}
          disabled={resendIn > 0 || pending || !sessionId}
          className="inline-flex h-12 items-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover disabled:cursor-not-allowed disabled:bg-ink-600"
        >
          {pending
            ? "Sending…"
            : resendIn > 0
              ? `Resend in ${resendIn}s`
              : "Resend the link"}
        </button>
        <p className="text-sm text-foreground-tertiary">
          Keep this tab open while you check your inbox.
        </p>
      </div>

      {message && (
        <p
          className="mt-4 text-sm text-foreground-secondary"
          aria-live="polite"
        >
          {message}
        </p>
      )}

      <dl className="mt-16 grid gap-10 border-t border-border-subtle pt-10 sm:grid-cols-2">
        <div>
          <dt className="eyebrow mb-3">Not in your inbox?</dt>
          <dd className="text-foreground-secondary leading-relaxed">
            Check spam or promotions if the email doesn&rsquo;t appear right
            away.
          </dd>
        </div>
        <div>
          <dt className="eyebrow mb-3">Cleanest handoff</dt>
          <dd className="text-foreground-secondary leading-relaxed">
            Use the same browser and device so the link brings you right back
            here.
          </dd>
        </div>
      </dl>
    </div>
  );
}
