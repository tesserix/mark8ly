import Link from "next/link";

import { getServerSessionContext } from "@/lib/auth/serverSession";

export default async function ReviewNotFound() {
  const { tenantName, email } = await getServerSessionContext();
  return (
          <main className="mx-auto w-full max-w-3xl">
        <h1 className="font-serif text-3xl text-foreground">
          Review not found
        </h1>
        <p className="mt-4 text-foreground-secondary">
          This review doesn&apos;t exist in your store, or you don&apos;t have
          access to it.
        </p>
        <Link
          href="/customers/reviews"
          className="mt-6 inline-flex items-center gap-2 rounded-sm bg-[color:var(--ink-900)] px-4 py-2 text-sm text-[color:var(--paper-200)] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          Back to reviews
        </Link>
      </main>
  );
}
