"use client";

// PaymentConfigForm — inline form for configuring a payment provider.
// Rendered inside ProviderCard's children slot when expanded.

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";

import { savePaymentConfig } from "@/app/settings/payments/actions";

interface PaymentConfigFormProps {
  provider: string;
  initialApiKey?: string;
  initialMode?: "test" | "live";
  initialIsActive?: boolean;
}

export function PaymentConfigForm({
  provider,
  initialApiKey = "",
  initialMode = "test",
  initialIsActive = false,
}: PaymentConfigFormProps) {
  const router = useRouter();
  const [apiKey, setApiKey] = useState(initialApiKey);
  const [secretKey, setSecretKey] = useState("");
  const [mode, setMode] = useState<"test" | "live">(initialMode);
  const [isActive, setIsActive] = useState(initialIsActive);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const [pending, startTransition] = useTransition();

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSuccess(false);

    startTransition(async () => {
      const result = await savePaymentConfig(provider, {
        api_key: apiKey,
        secret_key: secretKey || undefined,
        mode,
        is_active: isActive,
      });
      if (!result.ok) {
        setError(result.message);
        return;
      }
      setSuccess(true);
      setSecretKey("");
      router.refresh();
    });
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-5">
      <div className="grid gap-5 sm:grid-cols-2">
        <FieldGroup label="API key" htmlFor={`${provider}-api-key`}>
          <input
            id={`${provider}-api-key`}
            type="password"
            autoComplete="off"
            value={apiKey}
            onChange={(e) => { setApiKey(e.target.value); setSuccess(false); }}
            placeholder="pk_test_..."
            required
            disabled={pending}
            className="w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-[color:var(--paper-200)] px-3 py-2 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50"
          />
        </FieldGroup>

        <FieldGroup label="Secret key" htmlFor={`${provider}-secret-key`}>
          <input
            id={`${provider}-secret-key`}
            type="password"
            autoComplete="off"
            value={secretKey}
            onChange={(e) => { setSecretKey(e.target.value); setSuccess(false); }}
            placeholder="sk_test_..."
            disabled={pending}
            className="w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-[color:var(--paper-200)] px-3 py-2 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50"
          />
          <p className="text-xs text-[color:var(--ink-900)]/40 mt-1">
            Leave blank to keep the existing secret key.
          </p>
        </FieldGroup>
      </div>

      <div className="flex items-center gap-6">
        <FieldGroup label="Mode" htmlFor={`${provider}-mode`}>
          <select
            id={`${provider}-mode`}
            value={mode}
            onChange={(e) => { setMode(e.target.value as "test" | "live"); setSuccess(false); }}
            disabled={pending}
            className="rounded-[6px] border border-[color:var(--ink-900)]/10 bg-[color:var(--paper-200)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50"
          >
            <option value="test">Test mode</option>
            <option value="live">Live mode</option>
          </select>
        </FieldGroup>

        <label className="flex items-center gap-2 cursor-pointer select-none pt-5">
          <input
            type="checkbox"
            checked={isActive}
            onChange={(e) => { setIsActive(e.target.checked); setSuccess(false); }}
            disabled={pending}
            className="h-4 w-4 rounded border-[color:var(--ink-900)]/20 text-[color:var(--moss-700)] focus:ring-[color:var(--moss-700)]"
          />
          <span className="text-sm text-[color:var(--ink-900)]">Active</span>
        </label>
      </div>

      {error && (
        <div
          role="alert"
          className="rounded-[6px] border border-red-200 bg-red-50 px-4 py-2.5 text-sm text-red-800"
        >
          {error}
        </div>
      )}
      {success && (
        <div
          role="status"
          className="animate-in fade-in duration-300 rounded-[6px] border border-[color:var(--moss-700)]/20 bg-[color:var(--moss-700)]/5 px-4 py-2.5 text-sm text-[color:var(--moss-700)]"
        >
          Configuration saved.
        </div>
      )}

      <div className="flex justify-end">
        <button
          type="submit"
          disabled={pending || !apiKey.trim()}
          className="rounded-[6px] bg-[color:var(--ink-900)] px-5 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          {pending ? "Saving..." : "Save configuration"}
        </button>
      </div>
    </form>
  );
}

function FieldGroup({
  label,
  htmlFor,
  children,
}: {
  label: string;
  htmlFor: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <label
        htmlFor={htmlFor}
        className="block text-sm font-medium text-[color:var(--ink-900)]"
      >
        {label}
      </label>
      {children}
    </div>
  );
}
