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
        className="text-sm text-[color:var(--storefront-accent,theme(colors.moss.700))] underline-offset-2 hover:underline"
      >
        Have a promo code?
      </button>
    );
  }

  if (applied) {
    return (
      <div className="flex items-center justify-between rounded-md border border-[color:var(--storefront-accent,theme(colors.moss.700))]/25 bg-[color:var(--storefront-accent,theme(colors.moss.700))]/8 px-3 py-2">
        <div className="text-sm">
          <span className="font-mono font-medium text-[color:var(--storefront-accent,theme(colors.moss.700))]">
            {applied.code}
          </span>
          <span className="ml-2 text-[color:var(--storefront-text,var(--ink-900))]/60">
            {applied.free_shipping
              ? "Free shipping"
              : `-${currencyCode} ${applied.discount_amount}`}
          </span>
        </div>
        <button
          type="button"
          onClick={remove}
          aria-label="Remove coupon"
          className="text-xs text-[color:var(--storefront-text,var(--ink-900))]/60 hover:text-[color:var(--storefront-text,var(--ink-900))]/90"
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
          aria-invalid={error ? true : undefined}
          aria-describedby={error ? "coupon-code-error" : undefined}
          className="flex-1 rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface,#ffffff)] px-3 py-2 text-sm font-mono uppercase text-[color:var(--storefront-text,var(--ink-900))] placeholder:text-[color:var(--storefront-text,var(--ink-900))]/50 placeholder:normal-case focus-visible:border-[color:var(--storefront-accent,var(--moss-700))] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[color:var(--storefront-accent,var(--moss-700))]"
        />
        <button
          type="button"
          onClick={validate}
          disabled={loading || !code.trim()}
          className="rounded-md bg-[color:var(--storefront-text,var(--ink-900))] px-4 py-2.5 text-sm font-medium text-[color:var(--storefront-background,var(--paper-200))] transition hover:opacity-90 disabled:opacity-50"
        >
          {loading ? "..." : "Apply"}
        </button>
      </div>
      {error && (
        <p
          id="coupon-code-error"
          role="alert"
          className="text-xs text-[color:var(--storefront-danger)]"
        >
          {error}
        </p>
      )}
    </div>
  );
}
