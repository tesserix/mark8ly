import Link from "next/link";

// Minimal landing page. The real marketing site (blog, guides, help center)
// is deferred to Phase F Tier 2.
export default function HomePage() {
  return (
    <main className="min-h-screen flex items-center justify-center px-4 py-12">
      <div className="max-w-lg text-center">
        <h1 className="text-4xl font-semibold tracking-tight">Mark8ly</h1>
        <p className="mt-4 text-lg text-zinc-600 dark:text-zinc-400">
          A clean, modern way to launch your store.
        </p>
        <Link
          href="/onboarding"
          className="mt-8 inline-block bg-zinc-900 hover:bg-zinc-800 dark:bg-white dark:hover:bg-zinc-100 text-white dark:text-zinc-900 font-medium px-6 py-3 rounded-lg transition-colors"
        >
          Start your store
        </Link>
      </div>
    </main>
  );
}
