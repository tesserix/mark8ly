import type { ReactNode } from "react";

import { AdminShell } from "@/components/shell/AdminShell";
import { Toaster } from "@/components/feedback/Toaster";
import { getServerSessionContext } from "@/lib/auth/serverSession";

/**
 * Admin route-group layout.
 *
 * Owns the sidebar + topbar chrome for every authenticated admin surface.
 * Because this layout lives at the route-group level and persists across
 * navigations, the sidebar never unmounts — fixing the "flash and come
 * back" behaviour that happened when every page rendered its own
 * `<AdminShell>`.
 *
 * `loading.tsx` and `error.tsx` files inside this group render into the
 * `{children}` slot below, so suspense fallbacks and error boundaries
 * now appear inside the shell instead of replacing the whole page.
 *
 * Session context is fetched here (once per request); Next.js dedupes
 * the same call inside individual pages via React cache, so each page
 * can still read the session for its own data fetching without paying
 * the cost twice.
 */
export default async function AdminLayout({
  children,
}: {
  children: ReactNode;
}) {
  const { tenantName, email, role, memberships, tenantId, stores, currentStore } =
    await getServerSessionContext();

  return (
    <Toaster>
      <AdminShell
        tenantName={tenantName}
        userEmail={email}
        role={role}
        memberships={memberships}
        currentTenantId={tenantId}
        stores={stores}
        currentStoreId={currentStore?.id}
      >
        {children}
      </AdminShell>
    </Toaster>
  );
}
