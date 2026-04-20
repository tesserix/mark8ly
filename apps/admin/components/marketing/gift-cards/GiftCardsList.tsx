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

// Dot + label status badge matching the orders/campaigns visual language.
function statusBadge(status: string) {
  const dotClass =
    status === "active"
      ? "bg-[color:var(--moss-700)]"
      : status === "pending"
        ? "border border-[color:var(--moss-700)] bg-transparent"
        : status === "refunded"
          ? "bg-[color:var(--signal)]"
          : status === "depleted" || status === "disabled"
            ? "bg-[color:var(--ink-900)] opacity-40"
            : "border border-[color:var(--ink-900)]/25 bg-transparent";
  const label = status.charAt(0).toUpperCase() + status.slice(1);
  return (
    <span className="inline-flex items-center gap-2 text-sm text-foreground">
      <span
        aria-hidden="true"
        className={`inline-block h-2 w-2 rounded-full ${dotClass}`}
      />
      {label}
    </span>
  );
}

// Source label — not a badge, just a small uppercase tag since it's
// reference metadata, not an axis of state.
function sourceLabel(gc: AdminGiftCard) {
  const label = gc.purchased_via_storefront ? "Storefront" : "Admin";
  return (
    <span
      className="text-[11px] uppercase tracking-[0.12em] text-foreground-tertiary"
      title={gc.purchased_by_email ?? undefined}
    >
      {label}
    </span>
  );
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
      className="ml-1.5 inline-flex items-center rounded p-0.5 text-foreground-tertiary transition-colors hover:text-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
    >
      {copied ? (
        <Check className="size-3.5 text-[color:var(--moss-700)]" aria-hidden="true" />
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
      {/* Underline tabs — hairline baseline, tabular-nums count, same
          pattern as orders/returns/campaigns. */}
      <nav
        role="tablist"
        aria-label="Filter gift cards by status"
        className="flex flex-wrap items-center gap-x-1 gap-y-0 border-b border-border-subtle px-1"
      >
        {[
          { label: "All", value: undefined },
          { label: "Active", value: "active" },
          { label: "Pending", value: "pending" },
          { label: "Depleted", value: "depleted" },
          { label: "Disabled", value: "disabled" },
          { label: "Refunded", value: "refunded" },
        ].map((tab) => {
          const active = currentStatus === tab.value;
          return (
            <button
              key={tab.label}
              role="tab"
              aria-selected={active}
              onClick={() => setStatusFilter(tab.value)}
              className={
                "flex shrink-0 items-baseline gap-1 border-b-2 px-2.5 py-2.5 text-[11px] font-semibold uppercase tracking-[0.08em] transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] " +
                (active
                  ? "border-[color:var(--ink-900)] text-foreground"
                  : "border-transparent text-foreground-tertiary hover:text-foreground")
              }
            >
              {tab.label}
            </button>
          );
        })}
      </nav>

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
                <td className="py-3 px-3">{sourceLabel(gc)}</td>
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

      {/* Pagination — editorial: serif tabular-nums current page, moss
          underline, chevrons. Matches the orders/products pattern. */}
      {meta && meta.total_pages > 1 && (
        <nav
          className="flex items-center justify-between pt-4 text-xs text-foreground-tertiary"
          aria-label="Gift cards pagination"
        >
          <span className="tabular-nums">
            {meta.total} gift card{meta.total !== 1 ? "s" : ""}
          </span>
          <div className="flex items-center gap-4">
            {meta.page > 1 && (
              <Link
                href={`/marketing/gift-cards?page=${meta.page - 1}${currentStatus ? `&status=${currentStatus}` : ""}`}
                className="text-foreground-secondary underline-offset-4 hover:text-[color:var(--moss-700)] hover:underline"
              >
                ‹ Previous
              </Link>
            )}
            <span className="font-serif text-base tabular-nums text-foreground">
              <span className="underline underline-offset-[6px] decoration-[color:var(--moss-700)] decoration-[1.5px]">
                {meta.page}
              </span>
              <span className="ml-1 text-foreground-tertiary">/ {meta.total_pages}</span>
            </span>
            {meta.page < meta.total_pages && (
              <Link
                href={`/marketing/gift-cards?page=${meta.page + 1}${currentStatus ? `&status=${currentStatus}` : ""}`}
                className="text-foreground-secondary underline-offset-4 hover:text-[color:var(--moss-700)] hover:underline"
              >
                Next ›
              </Link>
            )}
          </div>
        </nav>
      )}
    </div>
  );
}
