import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getGiftCard } from "@/lib/api/marketplace-api";
import { notFound } from "next/navigation";

import { Breadcrumbs } from "@/components/layout";
import { formatMoney } from "@/lib/format";

interface GiftCardDetailPageProps {
  params: Promise<{ id: string }>;
}

function formatCurrency(amount: string, currency: string): string {
  return formatMoney(amount, currency);
}

// Transaction type — editorial uppercase tertiary label, not a pill.
// The amount column's sign + color carries the state; the type label
// is reference metadata, not a state axis.
function txnTypeLabel(type: string) {
  const labelMap: Record<string, string> = {
    purchase: "Purchase",
    redeem: "Redeem",
    refund: "Refund",
    adjustment: "Adjustment",
  };
  return (
    <span className="text-[11px] uppercase tracking-[0.12em] text-foreground-tertiary">
      {labelMap[type] ?? type}
    </span>
  );
}

// Status — dot + label matching the list page.
function statusDotLabel(status: string) {
  const dotClass =
    status === "active"
      ? "bg-[color:var(--moss-700)]"
      : status === "pending"
        ? "border border-[color:var(--moss-700)] bg-transparent"
        : status === "refunded"
          ? "bg-[color:var(--signal)]"
          : "bg-[color:var(--ink-900)] opacity-40";
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

export default async function GiftCardDetailPage({ params }: GiftCardDetailPageProps) {
  const { id } = await params;
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, userId, tenantId } = session;

  if (!currentStore) {
    notFound();
  }

  const response = await getGiftCard(currentStore.id, id, { userId, tenantId });
  if (!response) {
    notFound();
  }

  const gc = response.data;

  return (
    <main className="flex flex-col gap-8">
      <Breadcrumbs
        items={[
          { label: "Marketing", href: "/marketing/campaigns" },
          { label: "Gift cards", href: "/marketing/gift-cards" },
          { label: gc.code_display },
        ]}
      />

      <header className="flex flex-col gap-2">
        <p className="eyebrow">Gift card</p>
        <h1 className="font-mono text-2xl font-medium text-foreground">
          {gc.code_display}
        </h1>
      </header>

        {/* Card summary */}
        <div className="grid grid-cols-3 gap-6">
          <div className="flex flex-col gap-1">
            <span className="text-xs font-medium uppercase tracking-wider text-foreground-tertiary">
              Current Balance
            </span>
            <span className="font-serif text-3xl tabular-nums text-foreground">
              {formatCurrency(gc.current_balance, gc.currency_code)}
            </span>
          </div>
          <div className="flex flex-col gap-1">
            <span className="text-xs font-medium uppercase tracking-wider text-foreground-tertiary">
              Initial Balance
            </span>
            <span className="font-serif text-xl tabular-nums text-foreground-secondary">
              {formatCurrency(gc.initial_balance, gc.currency_code)}
            </span>
          </div>
          <div className="flex flex-col gap-1">
            <span className="text-xs font-medium uppercase tracking-wider text-foreground-tertiary">
              Status
            </span>
            <span>{statusDotLabel(gc.status)}</span>
          </div>
        </div>

        <hr className="border-border-subtle" />

        {/* Details */}
        <div className="grid grid-cols-2 gap-x-8 gap-y-3 text-sm">
          {gc.recipient_name && (
            <div>
              <span className="text-foreground-tertiary">Recipient:</span>{" "}
              <span className="text-foreground">{gc.recipient_name}</span>
            </div>
          )}
          {gc.recipient_email && (
            <div>
              <span className="text-foreground-tertiary">Recipient email:</span>{" "}
              <span className="text-foreground">{gc.recipient_email}</span>
            </div>
          )}
          {gc.sender_name && (
            <div>
              <span className="text-foreground-tertiary">Sender:</span>{" "}
              <span className="text-foreground">{gc.sender_name}</span>
            </div>
          )}
          {gc.expires_at && (
            <div>
              <span className="text-foreground-tertiary">Expires:</span>{" "}
              <span className="text-foreground">
                {new Date(gc.expires_at).toLocaleDateString()}
              </span>
            </div>
          )}
          {gc.message && (
            <div className="col-span-2">
              <span className="text-foreground-tertiary">Message:</span>{" "}
              <span className="text-foreground italic">&ldquo;{gc.message}&rdquo;</span>
            </div>
          )}
        </div>

        <hr className="border-border-subtle" />

        {/* Transaction ledger */}
        <h2 className="font-serif text-lg text-foreground">Transaction History</h2>
        {gc.transactions.length === 0 ? (
          <p className="text-sm text-foreground-tertiary" aria-live="polite">No transactions yet.</p>
        ) : (
          <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border-subtle text-left text-xs font-medium uppercase tracking-wider text-foreground-tertiary">
                <th className="pb-3 pr-4">Date</th>
                <th className="pb-3 pr-4">Type</th>
                <th className="pb-3 pr-4">Amount</th>
                <th className="pb-3 pr-4">Balance After</th>
                <th className="pb-3">Note</th>
              </tr>
            </thead>
            <tbody>
              {gc.transactions.map((txn) => (
                <tr key={txn.id} className="border-b border-border-subtle">
                  <td className="py-3 pr-4 text-foreground-tertiary">
                    {new Date(txn.created_at).toLocaleString()}
                  </td>
                  <td className="py-3 pr-4">{txnTypeLabel(txn.type)}</td>
                  <td className={`py-3 pr-4 font-serif tabular-nums ${
                    Number(txn.amount) < 0
                      ? "text-[color:var(--signal)]"
                      : "text-[color:var(--moss-700)]"
                  }`}>
                    {Number(txn.amount) > 0 ? "+" : ""}
                    {formatCurrency(txn.amount, gc.currency_code)}
                  </td>
                  <td className="py-3 pr-4 font-serif tabular-nums text-foreground">
                    {formatCurrency(txn.balance_after, gc.currency_code)}
                  </td>
                  <td className="py-3 text-foreground-tertiary">{txn.note ?? "\u2014"}</td>
                </tr>
              ))}
            </tbody>
          </table>
          </div>
        )}
      </main>
  );
}
