"use client";

import { useCallback, useState } from "react";
import {
  checkGiftCardBalance,
  type GiftCardBalanceResult,
} from "@/lib/api/checkout-api";

interface GiftCardInputProps {
  storeSlug: string;
  currencyCode: string;
  onApplied: (code: string, balance: string) => void;
  onRemoved: () => void;
}

function formatPrice(amount: number, currencyCode: string): string {
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: currencyCode,
    }).format(amount);
  } catch {
    return `${currencyCode} ${amount.toFixed(2)}`;
  }
}

export function GiftCardInput({
  storeSlug,
  currencyCode,
  onApplied,
  onRemoved,
}: GiftCardInputProps) {
  const [showInput, setShowInput] = useState(false);
  const [code, setCode] = useState("");
  const [balance, setBalance] = useState<GiftCardBalanceResult | null>(null);
  const [applied, setApplied] = useState(false);
  const [checking, setChecking] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleCheckBalance = useCallback(async () => {
    if (!code.trim() || checking) return;
    setChecking(true);
    setError(null);
    try {
      const result = await checkGiftCardBalance(storeSlug, code.trim());
      if (!result) {
        setError("Gift card not found or expired");
        setBalance(null);
      } else {
        setBalance(result);
      }
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to check balance");
      setBalance(null);
    } finally {
      setChecking(false);
    }
  }, [storeSlug, code, checking]);

  function handleApply() {
    if (!balance) return;
    setApplied(true);
    onApplied(code.trim(), balance.current_balance);
  }

  function handleRemove() {
    setApplied(false);
    setBalance(null);
    setCode("");
    setError(null);
    onRemoved();
  }

  if (applied && balance) {
    return (
      <div className="flex items-center justify-between rounded-md border border-[color:var(--storefront-accent,theme(colors.moss.700))]/25 bg-[color:var(--storefront-accent,theme(colors.moss.700))]/8 px-4 py-3">
        <span className="text-sm text-[color:var(--storefront-accent,theme(colors.moss.700))]">
          Gift card applied: up to{" "}
          {formatPrice(Number(balance.current_balance), balance.currency_code)}
        </span>
        <button
          type="button"
          onClick={handleRemove}
          aria-label="Remove gift card"
          className="rounded text-sm text-[color:var(--storefront-text,var(--ink-900))]/60 transition-colors hover:text-[color:var(--storefront-text,var(--ink-900))]/90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
        >
          Remove
        </button>
      </div>
    );
  }

  return (
    <div className="border-t border-[color:var(--storefront-text,var(--ink-900))]/15 pt-4">
      <button
        type="button"
        onClick={() => setShowInput((v) => !v)}
        className="rounded text-sm text-[color:var(--storefront-accent,theme(colors.moss.700))] transition-colors hover:text-[color:var(--storefront-accent,theme(colors.moss.800))] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
      >
        Have a gift card?
      </button>
      {showInput && (
        <div className="mt-3 flex flex-col gap-2">
          <label htmlFor="gift-card-code" className="sr-only">
            Gift card code
          </label>
          <div className="flex flex-wrap gap-2">
            <input
              id="gift-card-code"
              type="text"
              value={code}
              onChange={(e) => setCode(e.target.value)}
              placeholder="Enter gift card code"
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? "gift-card-code-error" : undefined}
              className="flex-1 rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface,#ffffff)] px-3 py-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] placeholder:text-[color:var(--storefront-text,var(--ink-900))]/50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
            />
            <button
              type="button"
              onClick={handleCheckBalance}
              disabled={checking || !code.trim()}
              className="rounded-md bg-[color:var(--storefront-text,var(--ink-900))] px-4 py-2.5 text-sm text-[color:var(--storefront-background,var(--paper-200))] transition-colors hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))] disabled:opacity-40"
            >
              {checking ? "Checking..." : "Check"}
            </button>
          </div>
          {error && (
            <p
              id="gift-card-code-error"
              role="alert"
              aria-live="polite"
              className="text-sm text-[color:var(--storefront-danger)]"
            >
              {error}
            </p>
          )}
          {balance && !applied && (
            <div className="flex items-center justify-between rounded-md bg-[color:var(--storefront-accent,theme(colors.moss.700))]/8 px-3 py-2">
              <span className="text-sm text-[color:var(--storefront-text,var(--ink-900))]">
                Balance:{" "}
                {formatPrice(
                  Number(balance.current_balance),
                  balance.currency_code,
                )}
              </span>
              <button
                type="button"
                onClick={handleApply}
                className="text-sm font-medium text-[color:var(--storefront-accent,theme(colors.moss.700))] transition-colors hover:text-[color:var(--storefront-accent,theme(colors.moss.800))]"
              >
                Apply
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
