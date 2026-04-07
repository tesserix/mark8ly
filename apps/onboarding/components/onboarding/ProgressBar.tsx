"use client";

import type { WizardStep } from "@/lib/store/wizard-store";

const STEPS: { key: WizardStep; label: string }[] = [
  { key: "email", label: "Email" },
  { key: "verify", label: "Verify" },
  { key: "business", label: "Store" },
];

interface Props {
  current: WizardStep;
}

/**
 * Horizontal progress bar with 3 visible stops. The "submitting" and "done"
 * steps don't get their own dot — submitting fills the bar to 100% and
 * done is a redirect.
 */
export function ProgressBar({ current }: Props) {
  const currentIndex = STEPS.findIndex((s) => s.key === current);
  const fillIndex =
    current === "submitting" || current === "done" ? STEPS.length - 1 : currentIndex;

  return (
    <div className="w-full">
      <div className="flex items-center justify-between mb-2">
        {STEPS.map((s, i) => {
          const reached = i <= fillIndex;
          return (
            <div key={s.key} className="flex flex-col items-center flex-1">
              <div
                className={
                  "w-8 h-8 rounded-full flex items-center justify-center text-sm font-medium transition-colors " +
                  (reached
                    ? "bg-zinc-900 text-white dark:bg-white dark:text-zinc-900"
                    : "bg-zinc-200 text-zinc-500 dark:bg-zinc-800")
                }
              >
                {i + 1}
              </div>
              <span
                className={
                  "mt-2 text-xs " +
                  (reached
                    ? "text-zinc-900 dark:text-white font-medium"
                    : "text-zinc-500")
                }
              >
                {s.label}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
