// apps/admin/app/products/new/page.tsx
//
// Real New Product form — wires the shared ProductForm in "create" mode.
// M7b simple-product round-trip only; variants, media, SEO, rich text are
// deferred to M7c + follow-ups per the plan.

import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { listCategories } from "@/lib/api/marketplace-api";
import { ProductForm } from "@/components/products/ProductForm";

export default async function NewProductPage() {
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, role, userId, tenantId } = session;

  if (!currentStore) {
    return (
      <AdminShell tenantName={tenantName} userEmail={email}>
        <main className="mx-auto w-full max-w-3xl">
          <h1 className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-3xl text-[color:var(--ink-900)]">
            No store selected
          </h1>
          <p className="mt-4 text-[color:var(--ink-900)] opacity-70">
            Set up a store before creating products.
          </p>
        </main>
      </AdminShell>
    );
  }

  const categories = await listCategories(currentStore.id, {
    userId,
    tenantId,
  });

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="mx-auto w-full max-w-3xl">
        <ProductForm
          mode="create"
          storeId={currentStore.id}
          categories={categories}
          currencyCode={currentStore.currency_code}
          canDelete={false}
          canArchive={role === "owner" || role === "admin"}
          session={{ userId, tenantId }}
        />
      </main>
    </AdminShell>
  );
}
