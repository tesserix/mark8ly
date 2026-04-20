"use client";

// PaymentConfigForm — inline form for configuring a payment provider.
// Rendered inside ProviderCard's children slot when expanded.

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";

import { savePaymentConfig } from "@/app/(admin)/settings/payments/actions";

interface PaymentConfigFormProps {
  provider: string;
  initialApiKey?: string;
  initialMode?: "test" | "live";
  initialIsActive?: boolean;
  // True when the server already has a dedicated webhook secret stored
  // for this provider. Controls the help text on the field — "keep
  // existing" vs "optional, falls back to API secret".
  hasWebhookSecret?: boolean;
}

export function PaymentConfigForm({
  provider,
  initialApiKey = "",
  initialMode = "test",
  initialIsActive = false,
  hasWebhookSecret = false,
}: PaymentConfigFormProps) {
  const router = useRouter();
  const [apiKey, setApiKey] = useState(initialApiKey);
  const [secretKey, setSecretKey] = useState("");
  // Webhook secret uses tri-state semantics:
  //   untouched (never changed)   → omit from payload (backend preserves)
  //   touched, non-empty          → set to the new value
  //   touched, empty              → clear the column (backend falls back to API secret)
  const [webhookSecret, setWebhookSecret] = useState("");
  const [webhookSecretTouched, setWebhookSecretTouched] = useState(false);
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
        // Only include webhook_secret when the user actually interacted
        // with the field. This preserves the existing stored value when
        // untouched and lets an empty-on-purpose submission clear it.
        ...(webhookSecretTouched ? { webhook_secret: webhookSecret } : {}),
        mode,
        is_active: isActive,
      });
      if (!result.ok) {
        setError(result.message);
        return;
      }
      setSuccess(true);
      setSecretKey("");
      setWebhookSecret("");
      setWebhookSecretTouched(false);
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
            className="w-full rounded-md border border-[color:var(--ink-900)]/10 bg-[color:var(--paper-200)] px-3 py-2 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50"
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
            className="w-full rounded-md border border-[color:var(--ink-900)]/10 bg-[color:var(--paper-200)] px-3 py-2 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50"
          />
          <p className="text-xs text-[color:var(--ink-900)]/40 mt-1">
            Leave blank to keep the existing secret key.
          </p>
        </FieldGroup>
      </div>

      <FieldGroup
        label="Webhook signing secret"
        htmlFor={`${provider}-webhook-secret`}
      >
        <input
          id={`${provider}-webhook-secret`}
          type="password"
          autoComplete="off"
          value={webhookSecret}
          onChange={(e) => {
            setWebhookSecret(e.target.value);
            setWebhookSecretTouched(true);
            setSuccess(false);
          }}
          placeholder={webhookSecretPlaceholder(provider)}
          disabled={pending}
          className="w-full rounded-md border border-[color:var(--ink-900)]/10 bg-[color:var(--paper-200)] px-3 py-2 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/30 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50"
        />
        <p className="text-xs text-[color:var(--ink-900)]/40 mt-1">
          {webhookSecretHelpText(provider, hasWebhookSecret)}
        </p>
      </FieldGroup>

      <div className="flex items-center gap-6">
        <FieldGroup label="Mode" htmlFor={`${provider}-mode`}>
          <Select
            value={mode}
            onValueChange={(value) => {
              setMode(value as "test" | "live");
              setSuccess(false);
            }}
            disabled={pending}
          >
            <SelectTrigger id={`${provider}-mode`} className="w-40">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="test">Test mode</SelectItem>
              <SelectItem value="live">Live mode</SelectItem>
            </SelectContent>
          </Select>
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
          className="rounded-md border border-red-200 bg-red-50 px-4 py-2.5 text-sm text-red-800"
        >
          {error}
        </div>
      )}
      {success && (
        <div
          role="status"
          className="animate-in fade-in duration-300 rounded-md border border-[color:var(--moss-700)]/20 bg-[color:var(--moss-700)]/5 px-4 py-2.5 text-sm text-[color:var(--moss-700)]"
        >
          Configuration saved.
        </div>
      )}

      <div className="flex justify-end">
        <button
          type="submit"
          disabled={pending || !apiKey.trim()}
          className="rounded-md bg-[color:var(--ink-900)] px-5 py-2 text-sm font-medium text-white transition-colors hover:bg-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
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

function webhookSecretPlaceholder(provider: string): string {
  switch (provider.toLowerCase()) {
    case "stripe":
      return "whsec_...";
    case "razorpay":
      return "Razorpay webhook secret";
    case "paypal":
      return "PayPal webhook ID / secret";
    default:
      return "Webhook signing secret";
  }
}

function webhookSecretHelpText(
  provider: string,
  hasWebhookSecret: boolean,
): string {
  const p = provider.toLowerCase();
  if (hasWebhookSecret) {
    return "Leave blank to keep the existing webhook secret. Enter a new value to rotate it, or clear it to fall back to the API secret.";
  }
  if (p === "razorpay") {
    return "Optional. Configure a dedicated webhook secret in your Razorpay dashboard so rotating the API secret doesn't invalidate webhooks. Leave blank to use the API secret (legacy behaviour).";
  }
  if (p === "stripe") {
    return "Optional. Paste the signing secret from your Stripe webhook endpoint (starts with whsec_). Leave blank to use the API secret.";
  }
  return "Optional. When set, this secret is used for webhook signature verification instead of the API secret.";
}
