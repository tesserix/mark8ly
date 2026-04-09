// apps/admin/app/products/[id]/not-found.tsx

import Link from "next/link";

import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";

export default async function ProductNotFound() {
  const { tenantName, email } = await getServerSessionContext();
  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="mx-auto max-w-3xl px-8 py-16">
        <h1 className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-3xl text-[color:var(--ink-900)]">
          Product not found
        </h1>
        <p className="mt-4 text-[color:var(--ink-900)] opacity-70">
          This product doesn&apos;t exist in your catalogue, or you don&apos;t
          have access to it.
        </p>
        <Link
          href="/products"
          className="mt-6 inline-flex items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm text-[color:var(--paper-200)] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          Back to products
        </Link>
      </main>
    </AdminShell>
  );
}
