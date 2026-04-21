"use client";

import { useCallback, useState } from "react";
import { checkGiftCardBalance, type GiftCardBalanceResult } from "@/lib/api/checkout-api";

interface GiftCardBalanceFormProps {
  storeSlug: string;
}

export function GiftCardBalanceForm({ storeSlug }: GiftCardBalanceFormProps) {
  const [code, setCode] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<GiftCardBalanceResult | null>(null);

  const onSubmit = useCallback(
    async (e: React.FormEvent<HTMLFormElement>) => {
      e.preventDefault();
      if (!code.trim() || loading) return;
      setLoading(true);
      setError(null);
      setResult(null);

      try {
        const res = await checkGiftCardBalance(storeSlug, code.trim());
        if (!res) {
          setError("Gift card not found or expired.");
        } else {
          setResult(res);
        }
      } catch (err: unknown) {
        setError(err instanceof Error ? err.message : "Could not check balance.");
      } finally {
        setLoading(false);
      }
    },
    [code, storeSlug, loading],
  );

  return (
    <div className="space-y-8">
      <form onSubmit={onSubmit} className="space-y-4">
        <label htmlFor="gift-card-code" className="block">
          <span className="text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]">
            Gift card code
          </span>
          <input
            id="gift-card-code"
            type="text"
            value={code}
            onChange={(e) => {
              setCode(e.target.value);
              setResult(null);
              setError(null);
            }}
            placeholder="XXXX-XXXX-XXXX-XXXX-XXXX-XXXX-XX"
            autoComplete="off"
            autoCapitalize="characters"
            spellCheck={false}
            className="mt-2 block w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface,#ffffff)] px-4 py-3 font-mono text-sm uppercase tracking-wider text-[color:var(--storefront-text,var(--ink-900))] placeholder:text-[color:var(--storefront-text,var(--ink-900))]/40 focus-visible:border-[color:var(--storefront-accent,var(--moss-700))] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[color:var(--storefront-accent,var(--moss-700))]"
          />
        </label>

        <button
          type="submit"
          disabled={loading || !code.trim()}
          className="inline-flex items-center justify-center rounded-md bg-[color:var(--storefront-text,var(--ink-900))] px-6 py-3 text-sm font-medium text-[color:var(--storefront-background,var(--paper-200))] transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {loading ? "Checking..." : "Check balance"}
        </button>

        {error && (
          <p
            role="alert"
            aria-live="polite"
            className="rounded-md bg-[color:var(--signal)]/5 px-4 py-3 text-sm text-[color:var(--signal)]"
          >
            {error}
          </p>
        )}
      </form>

      {result && <BalanceResult result={result} />}
    </div>
  );
}

function BalanceResult({ result }: { result: GiftCardBalanceResult }) {
  const formatted = formatMoney(result.current_balance, result.currency_code);
  const expires = result.expires_at
    ? new Date(result.expires_at).toLocaleDateString(undefined, {
        year: "numeric",
        month: "long",
        day: "numeric",
      })
    : null;

  return (
    <section
      aria-live="polite"
      className="rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface,#ffffff)] p-6"
    >
      <p className="text-xs font-medium uppercase tracking-[0.16em] text-[color:var(--storefront-text,var(--ink-900))]/60">
        Current balance
      </p>
      <p className="mt-2 font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-4xl font-medium tracking-tight text-[color:var(--storefront-text,var(--ink-900))]">
        {formatted}
      </p>

      <dl className="mt-6 grid gap-3 text-sm sm:grid-cols-2">
        <div>
          <dt className="text-xs font-medium uppercase tracking-[0.14em] text-[color:var(--storefront-text,var(--ink-900))]/60">
            Code
          </dt>
          <dd className="mt-1 font-mono text-sm text-[color:var(--storefront-text,var(--ink-900))] break-all">
            {formatCodeDisplay(result.code)}
          </dd>
        </div>
        <div>
          <dt className="text-xs font-medium uppercase tracking-[0.14em] text-[color:var(--storefront-text,var(--ink-900))]/60">
            Status
          </dt>
          <dd className="mt-1 text-[color:var(--storefront-text,var(--ink-900))] capitalize">{result.status}</dd>
        </div>
        {expires && (
          <div className="sm:col-span-2">
            <dt className="text-xs font-medium uppercase tracking-[0.14em] text-[color:var(--storefront-text,var(--ink-900))]/60">
              Expires
            </dt>
            <dd className="mt-1 text-[color:var(--storefront-text,var(--ink-900))]">{expires}</dd>
          </div>
        )}
      </dl>

      <p className="mt-6 text-sm text-[color:var(--storefront-text,var(--ink-900))]/80">
        Apply this code at checkout to spend your balance.
      </p>
    </section>
  );
}

function formatMoney(amount: string, currency: string): string {
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency,
    }).format(Number(amount));
  } catch {
    return `${currency} ${amount}`;
  }
}

function formatCodeDisplay(code: string): string {
  // If already formatted with dashes, leave it.
  if (code.includes("-")) return code;
  // 26 chars → XXXX-XXXX-XXXX-XXXX-XXXX-XXXX-XX
  if (code.length === 26) {
    return `${code.slice(0, 4)}-${code.slice(4, 8)}-${code.slice(8, 12)}-${code.slice(12, 16)}-${code.slice(16, 20)}-${code.slice(20, 24)}-${code.slice(24, 26)}`;
  }
  return code;
}
