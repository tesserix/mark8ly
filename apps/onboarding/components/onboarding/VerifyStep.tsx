"use client";

import { useEffect, useRef, useState, useTransition } from "react";

import { useWizardStore } from "@/lib/store/wizard-store";
import { verifyCode, sendVerification } from "@/app/onboarding/actions";
import { signUp } from "@/lib/gip/signup";

const RESEND_COOLDOWN_SECONDS = 30;

/**
 * VerifyStep collects the 6-digit OTP. After successful verify it kicks
 * off the GIP signup in parallel so by the time the user fills out
 * step 3 the id_token is ready.
 */
export function VerifyStep() {
  const email = useWizardStore((s) => s.email);
  const sessionId = useWizardStore((s) => s.sessionId);
  const setStep = useWizardStore((s) => s.setStep);
  const setGip = useWizardStore((s) => s.setGip);

  const [digits, setDigits] = useState<string[]>(["", "", "", "", "", ""]);
  const [error, setError] = useState<string | null>(null);
  const [resendIn, setResendIn] = useState(0);
  const [pending, startTransition] = useTransition();

  const inputRefs = useRef<(HTMLInputElement | null)[]>([]);

  // Resend cooldown ticker.
  useEffect(() => {
    if (resendIn <= 0) return;
    const t = setTimeout(() => setResendIn(resendIn - 1), 1000);
    return () => clearTimeout(t);
  }, [resendIn]);

  // Autofocus first digit on mount.
  useEffect(() => {
    inputRefs.current[0]?.focus();
  }, []);

  function handleDigitChange(idx: number, value: string) {
    // Allow only digits and at most 1 char (paste handled separately).
    const cleaned = value.replace(/\D/g, "").slice(0, 1);
    const next = [...digits];
    next[idx] = cleaned;
    setDigits(next);

    // Auto-advance to the next input.
    if (cleaned && idx < 5) {
      inputRefs.current[idx + 1]?.focus();
    }
    // Auto-submit when all 6 digits are filled.
    if (cleaned && idx === 5 && next.every((d) => d.length === 1)) {
      submit(next.join(""));
    }
  }

  function handlePaste(e: React.ClipboardEvent<HTMLInputElement>) {
    e.preventDefault();
    const pasted = e.clipboardData.getData("text").replace(/\D/g, "").slice(0, 6);
    if (!pasted) return;
    const next = pasted.split("").concat(Array(6 - pasted.length).fill(""));
    setDigits(next);
    if (pasted.length === 6) {
      submit(pasted);
    } else {
      inputRefs.current[pasted.length]?.focus();
    }
  }

  function handleKeyDown(idx: number, e: React.KeyboardEvent<HTMLInputElement>) {
    // Backspace on empty input → focus previous.
    if (e.key === "Backspace" && !digits[idx] && idx > 0) {
      inputRefs.current[idx - 1]?.focus();
    }
  }

  function submit(code: string) {
    setError(null);
    startTransition(async () => {
      // Step 1: verify OTP via platform-api.
      const v = await verifyCode(sessionId, code);
      if (!v.ok) {
        setError(v.message);
        // Reset digits so the user can try again.
        setDigits(["", "", "", "", "", ""]);
        inputRefs.current[0]?.focus();
        return;
      }

      // Step 2: kick off GIP signup. We do this AFTER OTP verify so we
      // know the email is real before creating a GIP user.
      try {
        const gip = await signUp(email);
        setGip(gip);
      } catch (err) {
        // GIP signup failure is recoverable — show the error and stay
        // on this step. Common cause: email already taken in GIP, which
        // means the user already has an account.
        setError(
          err instanceof Error
            ? `Account creation failed: ${err.message}`
            : "Account creation failed",
        );
        return;
      }

      setStep("business");
    });
  }

  function resend() {
    if (resendIn > 0) return;
    setError(null);
    setResendIn(RESEND_COOLDOWN_SECONDS);
    startTransition(async () => {
      const r = await sendVerification(sessionId);
      if (!r.ok) {
        setError(r.message);
        setResendIn(0);
      }
    });
  }

  return (
    <div>
      <h1 className="text-2xl font-semibold tracking-tight mb-2">
        Check your email
      </h1>
      <p className="text-sm text-zinc-500 dark:text-zinc-400 mb-6">
        We sent a 6-digit code to <strong>{email}</strong>.
      </p>

      <div className="flex gap-2 justify-center">
        {digits.map((d, i) => (
          <input
            key={i}
            ref={(el) => {
              inputRefs.current[i] = el;
            }}
            type="text"
            inputMode="numeric"
            maxLength={1}
            value={d}
            onChange={(e) => handleDigitChange(i, e.target.value)}
            onKeyDown={(e) => handleKeyDown(i, e)}
            onPaste={i === 0 ? handlePaste : undefined}
            disabled={pending}
            className="w-12 h-14 text-center text-2xl font-semibold rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 text-zinc-900 dark:text-zinc-100 focus:outline-none focus:ring-2 focus:ring-zinc-900 dark:focus:ring-white"
          />
        ))}
      </div>

      {error && (
        <p className="mt-4 text-sm text-red-600 text-center" role="alert">
          {error}
        </p>
      )}

      <div className="mt-6 text-center text-sm text-zinc-500">
        Didn't receive it?{" "}
        <button
          onClick={resend}
          disabled={resendIn > 0 || pending}
          className="text-zinc-900 dark:text-white font-medium underline disabled:no-underline disabled:text-zinc-400"
        >
          {resendIn > 0 ? `Resend in ${resendIn}s` : "Resend code"}
        </button>
      </div>
    </div>
  );
}
