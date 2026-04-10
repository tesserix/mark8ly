// apps/admin/app/products/[id]/page.tsx
//
// Product detail edit page. Fetches the product + categories server-side
// and renders the shared ProductForm in "edit" mode.

import { notFound } from "next/navigation";

import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getProduct, listCategories } from "@/lib/api/marketplace-api";
import { ProductForm } from "@/components/products/ProductForm";

interface PageProps {
  params: Promise<{ id: string }>;
}

export default async function ProductDetailPage({ params }: PageProps) {
  const { id } = await params;
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, role, userId, tenantId } = session;

  if (!currentStore) {
    notFound();
  }

  const [product, categories] = await Promise.all([
    getProduct(currentStore.id, id, { userId, tenantId }),
    listCategories(currentStore.id, { userId, tenantId }),
  ]);

  if (!product) {
    notFound();
  }

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="mx-auto max-w-3xl px-8 py-8">
        <ProductForm
          mode="edit"
          storeId={currentStore.id}
          initialProduct={product}
          categories={categories}
          currencyCode={currentStore.currency_code}
          canDelete={role === "owner"}
          canArchive={role === "owner" || role === "admin"}
          session={{ userId, tenantId }}
          storeSlug={currentStore.slug}
        />
      </main>
    </AdminShell>
  );
}
