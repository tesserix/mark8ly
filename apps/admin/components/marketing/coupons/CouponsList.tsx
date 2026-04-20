"use client";

import Link from "next/link";
import { useCallback, useState } from "react";
import { Check, Copy } from "lucide-react";
import type { AdminCoupon } from "@/lib/api/coupons-api";

interface CouponsListProps {
  coupons: AdminCoupon[];
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

function statusBadge(status: AdminCoupon["status"]) {
  const colors: Record<string, string> = {
    active: "bg-moss-700/10 text-moss-700",
    disabled: "bg-ink-100 text-ink-500",
    expired: "bg-[color:var(--signal)]/10 text-[color:var(--signal)]",
  };
  return (
    <span
      className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors[status] ?? "bg-ink-100 text-ink-500"}`}
    >
      {status}
    </span>
  );
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  const copy = useCallback(() => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  }, [text]);
  return (
    <button
      type="button"
      onClick={(e) => {
        e.stopPropagation();
        copy();
      }}
      aria-label={`Copy ${text}`}
      title="Copy code"
      className="ml-1.5 inline-flex items-center rounded p-0.5 text-ink-400 transition hover:text-ink-700"
    >
      {copied ? (
        <Check className="size-3.5 text-moss-700" aria-hidden="true" />
      ) : (
        <Copy className="size-3.5" aria-hidden="true" />
      )}
    </button>
  );
}

export function CouponsList({ coupons }: CouponsListProps) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-sm">
        <thead>
          <tr className="border-b border-[color:var(--ink-900)]/15 text-xs font-medium uppercase tracking-wider text-foreground-tertiary">
            <th className="pb-3 pl-4 pr-3">Code</th>
            <th className="pb-3 px-3">Title</th>
            <th className="pb-3 px-3">Type</th>
            <th className="pb-3 px-3">Value</th>
            <th className="pb-3 px-3">Used</th>
            <th className="pb-3 px-3">Status</th>
            <th className="pb-3 px-3 pr-4">Expires</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-[color:var(--ink-900)]/10">
          {coupons.map((c) => (
            <tr
              key={c.id}
              className="group transition-colors hover:bg-[color:var(--ink-900)]/[0.03] focus-within:bg-[color:var(--ink-900)]/[0.03]"
            >
              <td className="py-3 pl-4 pr-3">
                <span className="inline-flex items-center">
                  <Link
                    href={`/marketing/coupons/${c.id}`}
                    className="rounded-sm font-mono text-sm font-medium text-foreground underline-offset-2 transition-colors group-hover:text-[color:var(--moss-700)] group-hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
                  >
                    {c.code}
                  </Link>
                  <CopyButton text={c.code} />
                </span>
              </td>
              <td className="py-3 px-3 text-foreground">{c.title}</td>
              <td className="py-3 px-3 text-foreground-secondary">{formatType(c.type)}</td>
              <td className="py-3 px-3 font-mono tabular-nums text-foreground">
                {c.type === "percentage"
                  ? `${c.value}%`
                  : c.type === "free_shipping"
                    ? "—"
                    : `${c.currency_code ?? ""} ${c.value}`}
              </td>
              <td className="py-3 px-3 tabular-nums text-foreground-secondary">
                {c.usage_count}
                {c.usage_limit != null ? ` / ${c.usage_limit}` : ""}
              </td>
              <td className="py-3 px-3">{statusBadge(c.status)}</td>
              <td className="py-3 px-3 pr-4 text-foreground-secondary">
                {c.ends_at
                  ? new Date(c.ends_at).toLocaleDateString()
                  : "No expiry"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
