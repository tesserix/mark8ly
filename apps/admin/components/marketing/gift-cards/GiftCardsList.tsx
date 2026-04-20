"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useState } from "react";
import { Check, Copy } from "lucide-react";
import type { AdminGiftCard, ListProductsMeta } from "@/lib/api/marketplace-api";

interface GiftCardsListProps {
  giftCards: AdminGiftCard[];
  meta?: ListProductsMeta;
  currentStatus?: string;
}

function statusBadge(status: string) {
  const base = "inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium capitalize";
  switch (status) {
    case "active":
      return <span className={`${base} bg-moss-700/10 text-moss-700`}>Active</span>;
    case "pending":
      return <span className={`${base} bg-amber-100 text-amber-800`}>Pending</span>;
    case "depleted":
      return <span className={`${base} bg-ink-100 text-ink-600`}>Depleted</span>;
    case "disabled":
      return <span className={`${base} bg-ink-100 text-ink-500`}>Disabled</span>;
    case "refunded":
      return <span className={`${base} bg-[color:var(--signal)]/10 text-[color:var(--signal)]`}>Refunded</span>;
    default:
      return <span className={`${base} bg-ink-100 text-ink-600`}>{status}</span>;
  }
}

function sourceBadge(gc: AdminGiftCard) {
  const base = "inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium";
  if (gc.purchased_via_storefront) {
    return (
      <span className={`${base} bg-moss-700/10 text-moss-700`} title={gc.purchased_by_email ?? undefined}>
        Storefront
      </span>
    );
  }
  return <span className={`${base} bg-ink-100 text-ink-600`}>Admin</span>;
}

function formatCurrency(amount: string, currency: string): string {
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency,
    }).format(Number(amount));
  } catch {
    return `${currency} ${amount}`;
  }
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

export function GiftCardsList({ giftCards, meta, currentStatus }: GiftCardsListProps) {
  const router = useRouter();
  const searchParams = useSearchParams();

  function setStatusFilter(status: string | undefined) {
    const params = new URLSearchParams(searchParams.toString());
    if (status) {
      params.set("status", status);
    } else {
      params.delete("status");
    }
    params.set("page", "1");
    router.push(`/marketing/gift-cards?${params.toString()}`);
  }

  return (
    <div className="flex flex-col gap-4">
      {/* Status filter tabs */}
      <div className="flex gap-3 border-b border-ink-900/10 pb-2" role="tablist" aria-label="Filter gift cards by status">
        {[
          { label: "All", value: undefined },
          { label: "Active", value: "active" },
          { label: "Pending payment", value: "pending" },
          { label: "Depleted", value: "depleted" },
          { label: "Disabled", value: "disabled" },
          { label: "Refunded", value: "refunded" },
        ].map((tab) => (
          <button
            key={tab.label}
            role="tab"
            aria-selected={currentStatus === tab.value}
            onClick={() => setStatusFilter(tab.value)}
            className={`text-sm font-medium transition-colors ${
              currentStatus === tab.value
                ? "text-ink-900 border-b-2 border-ink-900 -mb-[9px] pb-2"
                : "text-ink-900/50 hover:text-ink-900"
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Table */}
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-[color:var(--ink-900)]/15 text-left text-xs font-medium uppercase tracking-wider text-foreground-tertiary">
              <th className="pb-3 pl-4 pr-3">Code</th>
              <th className="pb-3 px-3">Balance</th>
              <th className="pb-3 px-3">Initial</th>
              <th className="pb-3 px-3">Status</th>
              <th className="pb-3 px-3">Source</th>
              <th className="pb-3 px-3">Recipient</th>
              <th className="pb-3 px-3 pr-4">Created</th>
            </tr>
          </thead>
          <tbody>
            {giftCards.map((gc) => (
              <tr
                key={gc.id}
                className="group border-b border-[color:var(--ink-900)]/5 transition-colors hover:bg-[color:var(--ink-900)]/[0.03] focus-within:bg-[color:var(--ink-900)]/[0.03]"
              >
                <td className="py-3 pl-4 pr-3">
                  <span className="inline-flex items-center">
                    <Link
                      href={`/marketing/gift-cards/${gc.id}`}
                      className="rounded-sm font-mono text-sm text-foreground transition-colors group-hover:text-[color:var(--moss-700)] group-hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
                    >
                      {gc.code_display}
                    </Link>
                    <CopyButton text={gc.code_display} />
                  </span>
                </td>
                <td className="py-3 px-3 font-serif tabular-nums text-foreground">
                  {formatCurrency(gc.current_balance, gc.currency_code)}
                </td>
                <td className="py-3 px-3 tabular-nums text-foreground-secondary">
                  {formatCurrency(gc.initial_balance, gc.currency_code)}
                </td>
                <td className="py-3 px-3">{statusBadge(gc.status)}</td>
                <td className="py-3 px-3">{sourceBadge(gc)}</td>
                <td className="py-3 px-3 text-foreground-secondary">
                  {gc.recipient_name ?? gc.recipient_email ?? "\u2014"}
                </td>
                <td className="py-3 px-3 pr-4 text-foreground-tertiary">
                  {new Date(gc.created_at).toLocaleDateString()}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      {meta && meta.total_pages > 1 && (
        <div className="flex items-center justify-between text-sm text-ink-500">
          <span>
            {meta.total} gift card{meta.total !== 1 ? "s" : ""}
          </span>
          <div className="flex gap-2">
            {meta.page > 1 && (
              <Link
                href={`/marketing/gift-cards?page=${meta.page - 1}${currentStatus ? `&status=${currentStatus}` : ""}`}
                className="text-moss-700 hover:underline"
              >
                Previous
              </Link>
            )}
            <span>
              Page {meta.page} of {meta.total_pages}
            </span>
            {meta.page < meta.total_pages && (
              <Link
                href={`/marketing/gift-cards?page=${meta.page + 1}${currentStatus ? `&status=${currentStatus}` : ""}`}
                className="text-moss-700 hover:underline"
              >
                Next
              </Link>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
