import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getGiftCard } from "@/lib/api/marketplace-api";
import { notFound } from "next/navigation";
import Link from "next/link";

interface GiftCardDetailPageProps {
  params: Promise<{ id: string }>;
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

function txnTypeBadge(type: string) {
  const base = "inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium";
  switch (type) {
    case "purchase":
      return <span className={`${base} bg-moss-700/10 text-moss-700`}>Purchase</span>;
    case "redeem":
      return <span className={`${base} bg-ink-900/10 text-ink-900`}>Redeem</span>;
    case "refund":
      return <span className={`${base} bg-ink-100 text-ink-600`}>Refund</span>;
    case "adjustment":
      return <span className={`${base} bg-ink-900/10 text-ink-900/70`}>Adjustment</span>;
    default:
      return <span className={`${base} bg-ink-900/10 text-ink-900`}>{type}</span>;
  }
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
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="flex flex-col gap-6 px-8 py-6">
        {/* Header */}
        <div className="flex items-center gap-3">
          <Link
            href="/marketing/gift-cards"
            className="text-sm text-ink-500 hover:text-ink-900"
          >
            Gift Cards
          </Link>
          <span className="text-ink-500">/</span>
          <span className="font-mono text-sm text-ink-900">{gc.code_display}</span>
        </div>

        {/* Card summary */}
        <div className="grid grid-cols-3 gap-6">
          <div className="flex flex-col gap-1">
            <span className="text-xs font-medium uppercase tracking-wider text-ink-500">
              Current Balance
            </span>
            <span className="font-serif text-3xl tabular-nums text-ink-900">
              {formatCurrency(gc.current_balance, gc.currency_code)}
            </span>
          </div>
          <div className="flex flex-col gap-1">
            <span className="text-xs font-medium uppercase tracking-wider text-ink-500">
              Initial Balance
            </span>
            <span className="font-serif text-xl tabular-nums text-ink-900/70">
              {formatCurrency(gc.initial_balance, gc.currency_code)}
            </span>
          </div>
          <div className="flex flex-col gap-1">
            <span className="text-xs font-medium uppercase tracking-wider text-ink-500">
              Status
            </span>
            <span className="text-sm capitalize text-ink-900">{gc.status}</span>
          </div>
        </div>

        <hr className="border-ink-900/10" />

        {/* Details */}
        <div className="grid grid-cols-2 gap-x-8 gap-y-3 text-sm">
          {gc.recipient_name && (
            <div>
              <span className="text-ink-500">Recipient:</span>{" "}
              <span className="text-ink-900">{gc.recipient_name}</span>
            </div>
          )}
          {gc.recipient_email && (
            <div>
              <span className="text-ink-500">Recipient email:</span>{" "}
              <span className="text-ink-900">{gc.recipient_email}</span>
            </div>
          )}
          {gc.sender_name && (
            <div>
              <span className="text-ink-500">Sender:</span>{" "}
              <span className="text-ink-900">{gc.sender_name}</span>
            </div>
          )}
          {gc.expires_at && (
            <div>
              <span className="text-ink-500">Expires:</span>{" "}
              <span className="text-ink-900">
                {new Date(gc.expires_at).toLocaleDateString()}
              </span>
            </div>
          )}
          {gc.message && (
            <div className="col-span-2">
              <span className="text-ink-500">Message:</span>{" "}
              <span className="text-ink-900 italic">&ldquo;{gc.message}&rdquo;</span>
            </div>
          )}
        </div>

        <hr className="border-ink-900/10" />

        {/* Transaction ledger */}
        <h2 className="font-serif text-lg text-ink-900">Transaction History</h2>
        {gc.transactions.length === 0 ? (
          <p className="text-sm text-ink-500" aria-live="polite">No transactions yet.</p>
        ) : (
          <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-ink-900/10 text-left text-xs font-medium uppercase tracking-wider text-ink-500">
                <th className="pb-3 pr-4">Date</th>
                <th className="pb-3 pr-4">Type</th>
                <th className="pb-3 pr-4">Amount</th>
                <th className="pb-3 pr-4">Balance After</th>
                <th className="pb-3">Note</th>
              </tr>
            </thead>
            <tbody>
              {gc.transactions.map((txn) => (
                <tr key={txn.id} className="border-b border-ink-900/5">
                  <td className="py-3 pr-4 text-ink-500">
                    {new Date(txn.created_at).toLocaleString()}
                  </td>
                  <td className="py-3 pr-4">{txnTypeBadge(txn.type)}</td>
                  <td className={`py-3 pr-4 font-serif tabular-nums ${
                    Number(txn.amount) < 0 ? "text-signal" : "text-moss-700"
                  }`}>
                    {Number(txn.amount) > 0 ? "+" : ""}
                    {formatCurrency(txn.amount, gc.currency_code)}
                  </td>
                  <td className="py-3 pr-4 font-serif tabular-nums text-ink-900">
                    {formatCurrency(txn.balance_after, gc.currency_code)}
                  </td>
                  <td className="py-3 text-ink-500">{txn.note ?? "\u2014"}</td>
                </tr>
              ))}
            </tbody>
          </table>
          </div>
        )}
      </main>
    </AdminShell>
  );
}
