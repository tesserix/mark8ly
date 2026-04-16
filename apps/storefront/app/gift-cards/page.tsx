import Link from "next/link";
import { StorefrontNav } from "@/components/StorefrontNav";

export default function GiftCardsPage() {
  return (
    <div className="min-h-screen bg-[color:var(--storefront-background,var(--paper-200))]">
      <StorefrontNav />
      <main id="main" className="mx-auto max-w-4xl px-6 py-16 sm:px-8">
        <header className="space-y-4">
          <p className="text-xs font-medium uppercase tracking-[0.18em] text-[color:var(--storefront-text,var(--ink-900))]/60">
            Gift cards
          </p>
          <h1 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-4xl font-medium tracking-tight text-[color:var(--storefront-text,var(--ink-900))] sm:text-5xl">
            Give something worth keeping.
          </h1>
          <p className="max-w-xl text-base leading-7 text-[color:var(--storefront-text,var(--ink-900))]/80">
            A gift card lets them choose exactly what they want — delivered
            instantly by email, redeemable any time, never expires unless you
            say so.
          </p>
        </header>

        <hr className="my-12 border-[color:var(--storefront-text,var(--ink-900))]/20" />

        <section
          aria-labelledby="gift-card-options"
          className="grid gap-6 sm:grid-cols-2"
        >
          <h2 id="gift-card-options" className="sr-only">
            Gift card options
          </h2>

          <Link
            href="/gift-cards/purchase"
            className="group flex flex-col justify-between gap-8 rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface,#ffffff)] p-8 transition-colors hover:border-[color:var(--storefront-accent,var(--moss-700))]"
          >
            <div className="space-y-3">
              <p className="text-xs font-medium uppercase tracking-[0.16em] text-[color:var(--storefront-text,var(--ink-900))]/60">
                Purchase
              </p>
              <h3 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-2xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
                Buy a gift card
              </h3>
              <p className="text-sm leading-6 text-[color:var(--storefront-text,var(--ink-900))]/80">
                Choose an amount, write a message, and we'll email it to the
                recipient immediately after payment clears.
              </p>
            </div>
            <span className="text-sm font-medium text-[color:var(--storefront-accent,theme(colors.moss.700))] underline-offset-4 group-hover:underline">
              Start purchase →
            </span>
          </Link>

          <Link
            href="/gift-cards/balance"
            className="group flex flex-col justify-between gap-8 rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface,#ffffff)] p-8 transition-colors hover:border-[color:var(--storefront-accent,var(--moss-700))]"
          >
            <div className="space-y-3">
              <p className="text-xs font-medium uppercase tracking-[0.16em] text-[color:var(--storefront-text,var(--ink-900))]/60">
                Balance
              </p>
              <h3 className="font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-2xl font-medium text-[color:var(--storefront-text,var(--ink-900))]">
                Check a balance
              </h3>
              <p className="text-sm leading-6 text-[color:var(--storefront-text,var(--ink-900))]/80">
                Hold a gift card already? Enter the code to see how much is
                left and when it expires.
              </p>
            </div>
            <span className="text-sm font-medium text-[color:var(--storefront-accent,theme(colors.moss.700))] underline-offset-4 group-hover:underline">
              Check balance →
            </span>
          </Link>
        </section>
      </main>
    </div>
  );
}
