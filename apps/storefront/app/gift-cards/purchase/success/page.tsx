import Link from "next/link";
import { StorefrontNav } from "@/components/StorefrontNav";

// Stripe redirects the customer back here with ?session_id=cs_... after
// a successful checkout. The real activation happens via webhook (the
// more trustworthy signal), but we still render a friendly confirmation
// here so the customer knows it worked.
//
// We don't poll the backend for the card — the webhook races the
// redirect, so a naive "lookup by session_id" would often 404. Instead
// we show an optimistic message and tell the recipient to check email.
interface PageProps {
  searchParams: Promise<{ session_id?: string }>;
}

export default async function GiftCardPurchaseSuccessPage({
  searchParams,
}: PageProps) {
  const { session_id } = await searchParams;

  return (
    <div className="min-h-screen bg-[color:var(--storefront-background,var(--paper-200))]">
      <StorefrontNav />
      <main id="main" className="mx-auto max-w-2xl px-6 py-20 sm:px-8">
        <header className="space-y-4">
          <p className="text-xs font-medium uppercase tracking-[0.18em] text-[color:var(--storefront-accent,theme(colors.moss.700))]">
            Payment received
          </p>
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-4xl font-medium tracking-tight text-ink-900">
            Your gift card is on its way.
          </h1>
          <p className="text-base leading-7 text-ink-700">
            Thanks for your purchase. The recipient will receive the gift card
            by email in the next few moments. If they don't see it, ask them to
            check spam.
          </p>
        </header>

        <hr className="my-10 border-ink-200" />

        <div className="space-y-4 text-sm text-ink-700">
          <p>
            A receipt has been sent to the email address you provided at
            checkout. You can keep that email for your records.
          </p>
          {session_id && (
            <p className="text-xs text-ink-500">
              Transaction reference:{" "}
              <span className="font-mono text-ink-700 break-all">
                {session_id}
              </span>
            </p>
          )}
        </div>

        <div className="mt-12 flex gap-3">
          <Link
            href="/"
            className="inline-flex items-center rounded-md bg-ink-900 px-5 py-2.5 text-sm font-medium text-paper-200 transition hover:bg-ink-800"
          >
            Continue shopping
          </Link>
          <Link
            href="/gift-cards/balance"
            className="inline-flex items-center rounded-md border border-ink-200 px-5 py-2.5 text-sm font-medium text-ink-900 transition hover:bg-ink-50"
          >
            Check balance
          </Link>
        </div>
      </main>
    </div>
  );
}
