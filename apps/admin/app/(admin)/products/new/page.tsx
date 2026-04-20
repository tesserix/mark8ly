// apps/admin/app/products/new/page.tsx
//
// Real New Product form — wires the shared ProductForm in "create" mode.
// M7b simple-product round-trip only; variants, media, SEO, rich text are
// deferred to M7c + follow-ups per the plan.

import { getServerSessionContext } from "@/lib/auth/serverSession";
import { listCategories } from "@/lib/api/marketplace-api";
import { Breadcrumbs } from "@/components/layout";
import { ProductForm } from "@/components/products/ProductForm";

export default async function NewProductPage() {
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, role, userId, tenantId } = session;

  if (!currentStore) {
    return (
      <main className="flex flex-col gap-4">
        <h1 className="font-serif text-3xl font-medium tracking-tight text-foreground">
          No store selected
        </h1>
        <p className="text-sm text-foreground-secondary">
          Set up a store before creating products.
        </p>
      </main>
    );
  }

  const categories = await listCategories(currentStore.id, {
    userId,
    tenantId,
  });

  return (
    <main className="flex flex-col gap-8">
      <Breadcrumbs
        items={[
          { label: "Products", href: "/products" },
          { label: "New product" },
        ]}
      />
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
  );
}
