"use client";

import { useCallback, useState } from "react";
import { Check, Copy } from "lucide-react";
import type { AdminCoupon } from "@/lib/api/coupons-api";

interface CouponDetailSummaryProps {
  coupon: AdminCoupon;
}

function formatType(type: AdminCoupon["type"]): string {
  switch (type) {
    case "percentage":
      return "Percentage";
    case "fixed_amount":
      return "Fixed amount";
    case "free_shipping":
      return "Free shipping";
    default:
      return type;
  }
}

function formatValue(coupon: AdminCoupon): string {
  if (coupon.type === "percentage") return `${coupon.value}%`;
  if (coupon.type === "free_shipping") return "Free shipping";
  return `${coupon.currency_code ?? ""} ${coupon.value}`;
}

export function CouponDetailSummary({ coupon }: CouponDetailSummaryProps) {
  return (
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <CodeField code={coupon.code} />
        <DetailField label="Type" value={formatType(coupon.type)} />
        <DetailField label="Value" value={formatValue(coupon)} mono />
        <DetailField label="Status" value={coupon.status} />
        <DetailField
          label="Used"
          value={
            coupon.usage_limit != null
              ? `${coupon.usage_count} / ${coupon.usage_limit}`
              : `${coupon.usage_count}`
          }
        />
        <DetailField label="Per customer" value={String(coupon.per_customer)} />
        <DetailField
          label="Starts"
          value={new Date(coupon.starts_at).toLocaleString()}
        />
        <DetailField
          label="Expires"
          value={
            coupon.ends_at
              ? new Date(coupon.ends_at).toLocaleString()
              : "No expiry"
          }
        />
        <DetailField
          label="Stackable"
          value={coupon.stackable ? "Yes" : "No"}
        />
        {coupon.min_purchase && (
          <DetailField
            label="Min purchase"
            value={`${coupon.currency_code ?? ""} ${coupon.min_purchase}`}
            mono
          />
        )}
        {coupon.max_discount && (
          <DetailField
            label="Max discount"
            value={`${coupon.currency_code ?? ""} ${coupon.max_discount}`}
            mono
          />
        )}
      </div>
      {coupon.description && (
        <div>
          <span className="text-xs font-medium uppercase tracking-wider text-ink-500">
            Description
          </span>
          <p className="mt-1 text-sm text-ink-700">{coupon.description}</p>
        </div>
      )}
    </div>
  );
}

function CodeField({ code }: { code: string }) {
  const [copied, setCopied] = useState(false);
  const copy = useCallback(() => {
    navigator.clipboard.writeText(code).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  }, [code]);
  return (
    <div>
      <span className="text-xs font-medium uppercase tracking-wider text-ink-500">
        Code
      </span>
      <p className="mt-1 inline-flex items-center gap-1.5 font-mono text-sm text-ink-900">
        {code}
        <button
          type="button"
          onClick={copy}
          aria-label={`Copy ${code}`}
          title="Copy code"
          className="inline-flex items-center rounded p-0.5 text-ink-400 transition hover:text-ink-700"
        >
          {copied ? (
            <Check className="size-3.5 text-moss-700" aria-hidden="true" />
          ) : (
            <Copy className="size-3.5" aria-hidden="true" />
          )}
        </button>
      </p>
    </div>
  );
}

function DetailField({
  label,
  value,
  mono = false,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div>
      <span className="text-xs font-medium uppercase tracking-wider text-ink-500">
        {label}
      </span>
      <p className={`mt-1 text-sm text-ink-900 ${mono ? "font-mono" : ""}`}>
        {value}
      </p>
    </div>
  );
}
