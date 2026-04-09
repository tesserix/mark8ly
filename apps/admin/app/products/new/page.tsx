// apps/admin/app/products/new/page.tsx
//
// Stub for the New Product detail form. The real detail page with
// title, description, categories, media, and variants lands in M7b.

import { AdminShell } from "@/components/shell/AdminShell";
import { ComingSoon } from "@/components/shell/ComingSoon";
import { getServerSessionContext } from "@/lib/auth/serverSession";

export default async function NewProductPage() {
  const { tenantName, email } = await getServerSessionContext();
  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <ComingSoon
        title="New product"
        description="The detail page with title, description, categories, media, and variants lands in the next slice."
        eta="M7b"
      />
    </AdminShell>
  );
}
