"use client";

// TaxJarConfigForm — inline form for configuring TaxJar API access.
// Only rendered for US stores where tax_strategy is "taxjar".

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";

import { saveTaxJarConfig } from "@/app/settings/tax/actions";

interface TaxJarConfigFormProps {
  existingApiKey?: string;
  existingMode?: string;
  isConfigured: boolean;
}

export function TaxJarConfigForm({
  existingApiKey,
  existingMode,
  isConfigured,
}: TaxJarConfigFormProps) {
  const router = useRouter();
  const [apiKey, setApiKey] = useState("");
  const [mode, setMode] = useState<"test" | "live">(
    (existingMode as "test" | "live") ?? "live",
  );
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const [pending, startTransition] = useTransition();

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSuccess(false);

    startTransition(async () => {
      const result = await saveTaxJarConfig({ api_key: apiKey, mode });
      if (!result.ok) {
        setError(result.message);
        return;
      }
      setSuccess(true);
      setApiKey("");
      router.refresh();
    });
  }

  const inputClass =
    "w-full rounded-[6px] border border-[color:var(--ink-900)]/10 bg-[color:var(--paper-200)] px-3 py-2 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50";

  return (
    <form onSubmit={handleSubmit} className="space-y-5">
      {isConfigured && existingApiKey && (
        <p className="text-sm text-[color:var(--ink-900)]/60 font-mono">
          Current key: {existingApiKey}
        </p>
      )}

      <div className="grid gap-5 sm:grid-cols-2">
        <div className="space-y-1.5">
          <label
            htmlFor="taxjar-api-key"
            className="block text-sm font-medium text-[color:var(--ink-900)]"
          >
            API Key
          </label>
          <input
            id="taxjar-api-key"
            type="password"
            autoComplete="off"
            value={apiKey}
            onChange={(e) => {
              setApiKey(e.target.value);
              setSuccess(false);
            }}
            placeholder={isConfigured ? "Enter new key to update" : "Your TaxJar API key"}
            required
            disabled={pending}
            className={inputClass}
          />
        </div>

        <div className="space-y-1.5">
          <label
            htmlFor="taxjar-mode"
            className="block text-sm font-medium text-[color:var(--ink-900)]"
          >
            Mode
          </label>
          <select
            id="taxjar-mode"
            value={mode}
            onChange={(e) => {
              setMode(e.target.value as "test" | "live");
              setSuccess(false);
            }}
            disabled={pending}
            className={inputClass}
          >
            <option value="test">Test mode (sandbox)</option>
            <option value="live">Live mode</option>
          </select>
        </div>
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
          TaxJar configuration saved.
        </div>
      )}

      <div className="flex justify-end">
        <button
          type="submit"
          disabled={pending || !apiKey.trim()}
          className="rounded-[6px] bg-[color:var(--ink-900)] px-5 py-2 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          {pending ? "Saving..." : "Save TaxJar configuration"}
        </button>
      </div>
    </form>
  );
}
