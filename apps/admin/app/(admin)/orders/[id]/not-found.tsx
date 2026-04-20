// app/orders/[id]/not-found.tsx

import Link from "next/link";

import { getServerSessionContext } from "@/lib/auth/serverSession";

export default async function OrderNotFound() {
  const { tenantName, email } = await getServerSessionContext();
  return (
    <main className="flex flex-col gap-6">
      <div className="flex flex-col gap-3">
        <h1 className="font-serif text-3xl font-medium tracking-tight text-foreground">
          Order not found
        </h1>
        <p className="max-w-prose text-sm text-foreground-secondary">
          This order doesn&apos;t exist in your store, or you don&apos;t have
          access to it.
        </p>
      </div>
      <Link
        href="/orders"
        className="inline-flex w-fit items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        Back to orders
      </Link>
    </main>
  );
}
