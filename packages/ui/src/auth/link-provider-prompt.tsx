"use client";

import { useState } from "react";

export interface LinkProviderPromptProps {
  email: string;
  onConfirm: (password: string) => Promise<void>;
  onCancel: () => void;
  error?: string | null;
  variant?: "admin" | "storefront";
}

export function LinkProviderPrompt({
  email,
  onConfirm,
  onCancel,
  error,
  variant = "admin",
}: LinkProviderPromptProps) {
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!password || submitting) return;
    setSubmitting(true);
    try {
      await onConfirm(password);
    } finally {
      setSubmitting(false);
    }
  }

  const isStorefront = variant === "storefront";
  const cardClasses = isStorefront
    ? "bg-[color:var(--storefront-surface,#fff)] text-[color:var(--storefront-text,var(--ink-900))]"
    : "bg-background-elevated text-foreground";
  const borderClasses = isStorefront
    ? "border-[color:var(--storefront-text,var(--ink-900))]/20"
    : "border-border";
  const accentRing = isStorefront
    ? "focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
    : "focus-visible:outline-moss-700";
  const primaryBtn = isStorefront
    ? "bg-[color:var(--storefront-accent,var(--ink-900))] text-[color:var(--storefront-on-accent,var(--paper-200))] hover:opacity-90"
    : "bg-primary text-primary-foreground hover:bg-primary-hover";
  const errorText = isStorefront
    ? "text-[color:var(--storefront-danger,#a3322a)]"
    : "text-danger";

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="link-provider-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
    >
      <div className={`w-full max-w-md rounded-lg p-6 shadow-2 ${cardClasses}`}>
        <h2
          id="link-provider-title"
          className="font-serif text-2xl font-medium tracking-tight"
        >
          Link Google to your existing account
        </h2>
        <p className="mt-3 text-sm leading-6 opacity-75">
          An account with <strong className="font-medium">{email}</strong>{" "}
          already exists. Enter your existing password to add Google sign-in
          to that account.
        </p>

        <form onSubmit={handleSubmit} className="mt-5 space-y-4">
          <div className="space-y-1.5">
            <label
              htmlFor="link-prompt-password"
              className="block text-sm font-medium"
            >
              Password
            </label>
            <input
              id="link-prompt-password"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              disabled={submitting}
              autoFocus
              className={`w-full rounded-md border px-3 py-2.5 text-base focus-visible:outline-2 focus-visible:outline-offset-2 ${borderClasses} ${accentRing} disabled:cursor-not-allowed disabled:opacity-50`}
            />
          </div>

          {error && (
            <p role="alert" className={`text-sm ${errorText}`}>
              {error}
            </p>
          )}

          <div className="flex gap-3">
            <button
              type="submit"
              disabled={submitting || !password}
              className={`inline-flex h-11 flex-1 items-center justify-center rounded-md px-6 text-sm font-medium transition-opacity ${primaryBtn} disabled:cursor-not-allowed disabled:opacity-50`}
            >
              {submitting ? "Linking…" : "Confirm and link"}
            </button>
            <button
              type="button"
              onClick={onCancel}
              disabled={submitting}
              className={`inline-flex h-11 items-center justify-center rounded-md border px-5 text-sm font-medium ${borderClasses} disabled:cursor-not-allowed disabled:opacity-50`}
            >
              Cancel
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
