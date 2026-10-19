/**
 * Top-of-page strip on Mark8ly's public demo stores (lib/demo-store.ts).
 * Server-rendered and above the merchant's own announcement bar, so it is
 * the first thing every page says and no merchant setting can hide it.
 */
export function DemoStoreBanner() {
  return (
    // Mark8ly's moss, not a theme variable: this is our label, not the
    // merchant's, and it must stay distinct from their announcement bar.
    <div role="note" className="bg-[#2D4A2B] text-white">
      <p className="mx-auto max-w-7xl px-4 py-2 text-center text-sm">
        <strong className="font-semibold">Demo store.</strong> A sample shop
        built on Mark8ly — no real payments are taken.{" "}
        <a
          href="https://mark8ly.com/"
          className="underline underline-offset-4 hover:no-underline"
        >
          Start your own store
        </a>
      </p>
    </div>
  );
}
