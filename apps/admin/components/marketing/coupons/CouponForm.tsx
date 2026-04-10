"use client";

import { useCallback, useState } from "react";
import { useRouter } from "next/navigation";
import type { CreateCouponBody } from "@/lib/api/coupons-api";

interface CouponFormProps {
  storeId: string;
  storeCurrency: string;
  onSubmit: (body: CreateCouponBody) => Promise<boolean>;
}

export function CouponForm({
  storeId,
  storeCurrency,
  onSubmit,
}: CouponFormProps) {
  const router = useRouter();
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Always-visible fields
  const [code, setCode] = useState("");
  const [type, setType] = useState<CreateCouponBody["type"]>("percentage");
  const [value, setValue] = useState("");
  const [endsAt, setEndsAt] = useState("");

  // Advanced fields
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [minPurchase, setMinPurchase] = useState("");
  const [maxDiscount, setMaxDiscount] = useState("");
  const [usageLimit, setUsageLimit] = useState("");
  const [perCustomer, setPerCustomer] = useState("1");
  const [stackable, setStackable] = useState(false);
  const [startsAt, setStartsAt] = useState("");

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      setError(null);
      setSubmitting(true);

      const body: CreateCouponBody = {
        code: code.trim().toUpperCase(),
        title: title.trim() || code.trim().toUpperCase(),
        type,
        value,
      };

      if (description.trim()) body.description = description.trim();
      if (type === "fixed_amount") body.currency_code = storeCurrency;
      if (endsAt) body.ends_at = new Date(endsAt).toISOString();
      if (startsAt) body.starts_at = new Date(startsAt).toISOString();
      if (minPurchase) body.min_purchase = minPurchase;
      if (maxDiscount) body.max_discount = maxDiscount;
      if (usageLimit) body.usage_limit = Number(usageLimit);
      if (perCustomer) body.per_customer = Number(perCustomer);
      body.stackable = stackable;

      try {
        const ok = await onSubmit(body);
        if (ok) {
          router.push("/marketing/coupons");
        } else {
          setError("Failed to create coupon. Please check your input.");
        }
      } catch {
        setError("An unexpected error occurred.");
      } finally {
        setSubmitting(false);
      }
    },
    [
      code, type, value, endsAt, title, description, minPurchase,
      maxDiscount, usageLimit, perCustomer, stackable, startsAt,
      storeCurrency, onSubmit, router,
    ],
  );

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      {error && (
        <div className="rounded-md border border-danger-200 bg-danger-50 px-4 py-3 text-sm text-danger-700">
          {error}
        </div>
      )}

      {/* Always visible: code, type, value, expiry */}
      <div className="grid gap-4 sm:grid-cols-2">
        <div>
          <label className="mb-1 block text-sm font-medium text-ink-700">
            Coupon code
          </label>
          <input
            type="text"
            required
            maxLength={50}
            value={code}
            onChange={(e) => setCode(e.target.value)}
            placeholder="e.g. SAVE20"
            className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm font-mono uppercase text-ink-900 placeholder:text-ink-400 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
          />
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium text-ink-700">
            Discount type
          </label>
          <select
            value={type}
            onChange={(e) =>
              setType(e.target.value as CreateCouponBody["type"])
            }
            className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
          >
            <option value="percentage">Percentage</option>
            <option value="fixed_amount">Fixed amount</option>
            <option value="free_shipping">Free shipping</option>
          </select>
        </div>
        {type !== "free_shipping" && (
          <div>
            <label className="mb-1 block text-sm font-medium text-ink-700">
              {type === "percentage" ? "Discount (%)" : `Amount (${storeCurrency})`}
            </label>
            <input
              type="number"
              required
              min="0"
              max={type === "percentage" ? "100" : undefined}
              step="0.01"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 placeholder:text-ink-400 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
            />
          </div>
        )}
        <div>
          <label className="mb-1 block text-sm font-medium text-ink-700">
            Expiry date
          </label>
          <input
            type="datetime-local"
            value={endsAt}
            onChange={(e) => setEndsAt(e.target.value)}
            className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
          />
          <p className="mt-1 text-xs text-ink-400">Leave empty for no expiry</p>
        </div>
      </div>

      {/* Advanced options toggle */}
      <button
        type="button"
        onClick={() => setShowAdvanced((v) => !v)}
        className="text-sm font-medium text-moss-700 underline-offset-2 hover:underline"
      >
        {showAdvanced ? "Hide advanced options" : "Advanced options"}
      </button>

      {showAdvanced && (
        <div className="grid gap-4 border-t border-ink-100 pt-4 sm:grid-cols-2">
          <div className="sm:col-span-2">
            <label className="mb-1 block text-sm font-medium text-ink-700">
              Title (internal label)
            </label>
            <input
              type="text"
              maxLength={200}
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="e.g. Spring sale 20% off"
              className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 placeholder:text-ink-400 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
            />
            <p className="mt-1 text-xs text-ink-400">
              Defaults to the coupon code if left empty
            </p>
          </div>
          <div className="sm:col-span-2">
            <label className="mb-1 block text-sm font-medium text-ink-700">
              Description
            </label>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={2}
              className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 placeholder:text-ink-400 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
            />
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-ink-700">
              Minimum purchase ({storeCurrency})
            </label>
            <input
              type="number"
              min="0"
              step="0.01"
              value={minPurchase}
              onChange={(e) => setMinPurchase(e.target.value)}
              className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
            />
          </div>
          {type === "percentage" && (
            <div>
              <label className="mb-1 block text-sm font-medium text-ink-700">
                Maximum discount ({storeCurrency})
              </label>
              <input
                type="number"
                min="0"
                step="0.01"
                value={maxDiscount}
                onChange={(e) => setMaxDiscount(e.target.value)}
                className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
              />
            </div>
          )}
          <div>
            <label className="mb-1 block text-sm font-medium text-ink-700">
              Total usage limit
            </label>
            <input
              type="number"
              min="1"
              value={usageLimit}
              onChange={(e) => setUsageLimit(e.target.value)}
              placeholder="Unlimited"
              className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 placeholder:text-ink-400 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
            />
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-ink-700">
              Uses per customer
            </label>
            <input
              type="number"
              min="1"
              value={perCustomer}
              onChange={(e) => setPerCustomer(e.target.value)}
              className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
            />
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-ink-700">
              Start date
            </label>
            <input
              type="datetime-local"
              value={startsAt}
              onChange={(e) => setStartsAt(e.target.value)}
              className="w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
            />
          </div>
          <div className="flex items-center gap-2 pt-6">
            <input
              type="checkbox"
              id="stackable"
              checked={stackable}
              onChange={(e) => setStackable(e.target.checked)}
              className="h-4 w-4 rounded border-ink-300 text-moss-700 focus:ring-moss-700"
            />
            <label htmlFor="stackable" className="text-sm text-ink-700">
              Allow stacking with other coupons
            </label>
          </div>
        </div>
      )}

      <hr className="border-ink-200" />

      <div className="flex items-center gap-3">
        <button
          type="submit"
          disabled={submitting}
          className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition hover:bg-ink-800 disabled:opacity-50"
        >
          {submitting ? "Creating..." : "Create coupon"}
        </button>
        <button
          type="button"
          onClick={() => router.push("/marketing/coupons")}
          className="rounded-md px-4 py-2 text-sm font-medium text-ink-500 transition hover:text-ink-700"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}
