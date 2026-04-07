import Link from "next/link";

/**
 * Marketing site footer — shared across the landing page and stub pages.
 * Lists the same link groups as the legacy mark8ly_backup footer so that
 * every footer link resolves (the actual destinations are placeholder
 * "coming soon" pages for now).
 */
export function Footer() {
  const year = new Date().getFullYear();

  return (
    <footer className="border-t border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-950">
      <div className="mx-auto max-w-6xl px-6 py-16">
        <div className="grid grid-cols-2 gap-10 md:grid-cols-4">
          <div>
            <h4 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100 mb-4">
              Product
            </h4>
            <ul className="space-y-3 text-sm text-zinc-600 dark:text-zinc-400">
              <li><Link href="/#features" className="hover:text-zinc-900 dark:hover:text-zinc-100">Features</Link></li>
              <li><Link href="/onboarding" className="hover:text-zinc-900 dark:hover:text-zinc-100">Get started</Link></li>
            </ul>
          </div>
          <div>
            <h4 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100 mb-4">
              Resources
            </h4>
            <ul className="space-y-3 text-sm text-zinc-600 dark:text-zinc-400">
              <li><Link href="/help" className="hover:text-zinc-900 dark:hover:text-zinc-100">Help center</Link></li>
              <li><Link href="/blog" className="hover:text-zinc-900 dark:hover:text-zinc-100">Blog</Link></li>
            </ul>
          </div>
          <div>
            <h4 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100 mb-4">
              Company
            </h4>
            <ul className="space-y-3 text-sm text-zinc-600 dark:text-zinc-400">
              <li><Link href="/about" className="hover:text-zinc-900 dark:hover:text-zinc-100">About</Link></li>
              <li><Link href="/contact" className="hover:text-zinc-900 dark:hover:text-zinc-100">Contact</Link></li>
            </ul>
          </div>
          <div>
            <h4 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100 mb-4">
              Legal
            </h4>
            <ul className="space-y-3 text-sm text-zinc-600 dark:text-zinc-400">
              <li><Link href="/privacy" className="hover:text-zinc-900 dark:hover:text-zinc-100">Privacy</Link></li>
              <li><Link href="/terms" className="hover:text-zinc-900 dark:hover:text-zinc-100">Terms</Link></li>
            </ul>
          </div>
        </div>

        <div className="mt-12 pt-8 border-t border-zinc-200 dark:border-zinc-800 flex flex-col sm:flex-row items-center justify-between gap-4">
          <p className="text-xs text-zinc-500">
            &copy; {year} Mark8ly by Tesserix. Crafted for people who make things.
          </p>
          <a
            href="mailto:hello@mark8ly.com"
            className="text-xs text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-100"
          >
            hello@mark8ly.com
          </a>
        </div>
      </div>
    </footer>
  );
}
