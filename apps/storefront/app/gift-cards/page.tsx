import { StorefrontNav } from "@/components/StorefrontNav";

export default function GiftCardsPage() {
  return (
    <div className="min-h-screen bg-[color:var(--paper-200)]">
      <StorefrontNav />
      <main
        id="main"
        className="mx-auto max-w-3xl px-6 py-12 sm:px-8"
      >
        <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-3xl font-medium text-[color:var(--ink-900)]">
          Gift Cards
        </h1>
        <p className="mt-4 text-base leading-7 text-[color:var(--ink-900)] opacity-70">
          Gift cards are coming soon. Check back later to purchase a gift card
          for someone special.
        </p>
      </main>
    </div>
  );
}
