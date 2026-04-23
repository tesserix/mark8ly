import { getServerSessionContext } from "@/lib/auth/serverSession";
import { listCategories } from "@/lib/api/marketplace-api";

import { AdminPage } from "@/components/layout";
import { CategoriesFeaturedManager } from "@/components/products/CategoriesFeaturedManager";

export default async function CategoriesPage() {
  const session = await getServerSessionContext();
  const { currentStore, userId, tenantId } = session;

  if (!currentStore) {
    return (
      <AdminPage
        eyebrow="Catalogue"
        title="Categories"
        description="Select a store to manage categories."
      >
        <p className="text-sm text-foreground-secondary">
          No store selected.
        </p>
      </AdminPage>
    );
  }

  const categories = await listCategories(currentStore.id, {
    userId,
    tenantId,
  });

  return (
    <AdminPage
      eyebrow="Catalogue"
      title="Categories"
      description="Feature up to a handful of categories — those will surface on the storefront products page. The rest remain browsable via the full /categories listing."
    >
      <CategoriesFeaturedManager
        storeId={currentStore.id}
        categories={categories}
      />
    </AdminPage>
  );
}
