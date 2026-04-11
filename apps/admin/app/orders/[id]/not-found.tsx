// app/orders/[id]/not-found.tsx

import Link from "next/link";

import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";

export default async function OrderNotFound() {
  const { tenantName, email } = await getServerSessionContext();
  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="mx-auto w-full max-w-3xl">
        <h1 className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-3xl text-[color:var(--ink-900)]">
          Order not found
        </h1>
        <p className="mt-4 text-[color:var(--ink-900)] opacity-70">
          This order doesn&apos;t exist in your store, or you don&apos;t have
          access to it.
        </p>
        <Link
          href="/orders"
          className="mt-6 inline-flex items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm text-[color:var(--paper-200)] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          Back to orders
        </Link>
      </main>
    </AdminShell>
  );
}
