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
    if (!code.trim()) return;
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
  }, [storeSlug, code]);

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
      <div className="flex items-center justify-between rounded-md border border-[color:var(--moss-700)]/20 bg-[color:var(--moss-700)]/5 px-4 py-3">
        <span className="text-sm text-[color:var(--moss-700)]">
          Gift card applied: up to{" "}
          {formatPrice(Number(balance.current_balance), balance.currency_code)}
        </span>
        <button
          type="button"
          onClick={handleRemove}
          className="text-sm text-[color:var(--ink-900)] opacity-50 transition-opacity hover:opacity-100"
        >
          Remove
        </button>
      </div>
    );
  }

  return (
    <div className="border-t border-[color:var(--ink-900)]/10 pt-4">
      <button
        type="button"
        onClick={() => setShowInput((v) => !v)}
        className="text-sm text-[color:var(--moss-700)] transition-opacity hover:opacity-80"
      >
        Have a gift card?
      </button>
      {showInput && (
        <div className="mt-3 flex flex-col gap-2">
          <div className="flex gap-2">
            <input
              type="text"
              value={code}
              onChange={(e) => setCode(e.target.value)}
              placeholder="Enter gift card code"
              className="flex-1 rounded-md border border-[color:var(--ink-900)]/15 bg-white px-3 py-2 text-sm text-[color:var(--ink-900)] placeholder:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
            />
            <button
              type="button"
              onClick={handleCheckBalance}
              disabled={checking || !code.trim()}
              className="rounded-md bg-[color:var(--ink-900)] px-3 py-2 text-sm text-[color:var(--paper-200)] transition-opacity disabled:opacity-40"
            >
              {checking ? "Checking..." : "Check"}
            </button>
          </div>
          {error && (
            <p className="text-sm text-[color:var(--danger,#8B2500)]">{error}</p>
          )}
          {balance && !applied && (
            <div className="flex items-center justify-between rounded-md bg-[color:var(--moss-700)]/5 px-3 py-2">
              <span className="text-sm text-[color:var(--ink-900)]">
                Balance:{" "}
                {formatPrice(
                  Number(balance.current_balance),
                  balance.currency_code,
                )}
              </span>
              <button
                type="button"
                onClick={handleApply}
                className="text-sm font-medium text-[color:var(--moss-700)] transition-opacity hover:opacity-80"
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
