import type { ReactNode } from "react";

import { AdminShell } from "@/components/shell/AdminShell";
import { Toaster } from "@/components/feedback/Toaster";
import { getAccountProfile } from "@/lib/api/settings-tier2-api";
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
  const { tenantName, email, role, memberships, tenantId, userId, stores, currentStore } =
    await getServerSessionContext();

  // Best-effort profile fetch so the header can show the user's avatar
  // and persisted name. Swallow errors — a missing profile just falls
  // back to the email-derived initial.
  const profile = userId && tenantId
    ? await getAccountProfile({ userId, tenantId, email }).catch(() => null)
    : null;

  return (
    <Toaster>
      <AdminShell
        tenantName={tenantName}
        userEmail={email}
        userName={profile?.name}
        userAvatarUrl={profile?.avatar_url}
        role={role}
        memberships={memberships}
        currentTenantId={tenantId}
        stores={stores}
        currentStoreId={currentStore?.id}
        userId={userId}
        tenantId={tenantId}
      >
        {children}
      </AdminShell>
    </Toaster>
  );
}
