"use client";

import { useCallback, useState } from "react";

interface CouponInputProps {
  storeSlug: string;
  customerEmail: string;
  subtotal: number;
  currencyCode: string;
  onApplied: (result: CouponValidateResult) => void;
  onRemoved: () => void;
}

interface CouponValidateResult {
  coupon_id: string;
  code: string;
  type: string;
  value: string;
  discount_amount: string;
  free_shipping: boolean;
  title: string;
}

// Use the same-origin proxy so the browser never needs
// MARKETPLACE_API_URL or X-Storefront-Key (both server-only).
function validateUrl(storeSlug: string): string {
  const qs = new URLSearchParams({ store: storeSlug }).toString();
  return `/api/checkout/coupons/validate?${qs}`;
}

export function CouponInput({
  storeSlug,
  customerEmail,
  subtotal,
  currencyCode,
  onApplied,
  onRemoved,
}: CouponInputProps) {
  const [open, setOpen] = useState(false);
  const [code, setCode] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [applied, setApplied] = useState<CouponValidateResult | null>(null);

  const validate = useCallback(async () => {
    if (!code.trim() || loading) return;
    setLoading(true);
    setError(null);

    try {
      const res = await fetch(validateUrl(storeSlug), {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Accept: "application/json",
        },
        body: JSON.stringify({
          code: code.trim(),
          customer_email: customerEmail,
          subtotal: subtotal.toFixed(2),
        }),
      });

      if (!res.ok) {
        const body = await res.json().catch(() => null);
        setError(body?.message ?? "Invalid coupon code");
        return;
      }

      const body = await res.json();
      const result = body.data as CouponValidateResult;
      setApplied(result);
      onApplied(result);
    } catch {
      setError("Failed to validate coupon");
    } finally {
      setLoading(false);
    }
  }, [code, storeSlug, customerEmail, subtotal, onApplied, loading]);

  const remove = useCallback(() => {
    setApplied(null);
    setCode("");
    setError(null);
    onRemoved();
  }, [onRemoved]);

  if (!open && !applied) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="text-sm text-moss-700 underline-offset-2 hover:underline"
      >
        Have a promo code?
      </button>
    );
  }

  if (applied) {
    return (
      <div className="flex items-center justify-between rounded-md border border-moss-200 bg-moss-50 px-3 py-2">
        <div className="text-sm">
          <span className="font-mono font-medium text-moss-700">
            {applied.code}
          </span>
          <span className="ml-2 text-ink-500">
            {applied.free_shipping
              ? "Free shipping"
              : `-${currencyCode} ${applied.discount_amount}`}
          </span>
        </div>
        <button
          type="button"
          onClick={remove}
          aria-label="Remove coupon"
          className="text-xs text-ink-500 hover:text-ink-700"
        >
          Remove
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      <label htmlFor="coupon-code" className="sr-only">
        Promo code
      </label>
      <div className="flex flex-wrap gap-2">
        <input
          id="coupon-code"
          type="text"
          value={code}
          onChange={(e) => setCode(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              validate();
            }
          }}
          placeholder="Enter promo code"
          className="flex-1 rounded-md border border-ink-200 bg-white px-3 py-2 text-sm font-mono uppercase text-ink-900 placeholder:text-ink-500 placeholder:normal-case focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
        />
        <button
          type="button"
          onClick={validate}
          disabled={loading || !code.trim()}
          className="rounded-md bg-ink-900 px-4 py-2.5 text-sm font-medium text-paper-200 transition hover:bg-ink-800 disabled:opacity-50"
        >
          {loading ? "..." : "Apply"}
        </button>
      </div>
      {error && <p className="text-xs text-signal-700">{error}</p>}
    </div>
  );
}
