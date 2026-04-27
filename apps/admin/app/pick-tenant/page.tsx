import type { Metadata } from "next";
import { headers } from "next/headers";
import { redirect } from "next/navigation";
import { BrandBar } from "@repo/ui/brand-bar";

import { listMemberTenants } from "@/lib/api/platform-api";
import { PickTenantForm } from "@/components/auth/PickTenantForm";
import { decidePickTenantOutcome } from "@/lib/auth/pick-tenant-decision";

export const metadata: Metadata = { title: "Pick a store" };

const PLATFORM_API_URL =
  process.env.PLATFORM_API_URL ?? "http://localhost:8086";

/**
 * Resolve the request's host to a tenant id when the host is a
 * `{slug}-admin.mark8ly.com` subdomain. Returns null for the canonical
 * admin host, custom domains, or when platform-api is unreachable.
 *
 * The pick-tenant page uses this to decide whether the host's slug is
 * one of the user's tenants — when it is, we auto-skip to /dashboard
 * (the single-tenant convenience path); when it isn't, we render the
 * picker no matter how many tenants the user owns. Without that
 * distinction, a single-tenant user on a wrong-slug subdomain
 * ping-pongs between /dashboard and /pick-tenant indefinitely
 * (middleware redirects on switch failure → page redirects to
 * /dashboard → middleware redirects again).
 */
async function tenantIdForHostSlug(
  hostHeader: string | null | undefined,
): Promise<string | null> {
  if (!hostHeader) return null;
  const host = hostHeader.split(":")[0] ?? "";
  const match = host.match(/^([^.]+)-admin\.mark8ly\.com$/);
  if (!match || !match[1]) return null;
  const slug = match[1];
  try {
    const res = await fetch(
      `${PLATFORM_API_URL}/internal/stores/by-slug/${encodeURIComponent(slug)}`,
      { cache: "no-store" },
    );
    if (!res.ok) return null;
    const body = (await res.json()) as { data?: { tenant_id?: string } };
    return body.data?.tenant_id ?? null;
  } catch {
    return null;
  }
}

/**
 * /pick-tenant — landing for users whose session tenant doesn't line
 * up with the requested URL.
 *
 * Reached via three paths:
 *   • Multi-tenant user finishes /login on the canonical host
 *     (admin.mark8ly.com) → routed here to choose a workspace.
 *   • Authenticated user navigates to {slug}-admin.mark8ly.com for a
 *     tenant they don't own → middleware tries to auto-switch, the
 *     switch fails for lack of membership, and middleware redirects
 *     here so the user can pick one of their actual tenants.
 *   • User clicks the in-app "Switch store" affordance.
 *
 * Decision matrix (host slug = the slug parsed off
 * `{slug}-admin.mark8ly.com`, null on the canonical host):
 *
 *  | tenants | host slug | action                                   |
 *  |---------|-----------|------------------------------------------|
 *  | 0       | any       | redirect("/login")                       |
 *  | 1       | null      | redirect("/dashboard") — fast path       |
 *  | 1       | matches   | redirect("/dashboard") — same host       |
 *  | 1       | mismatch  | render picker w/ "wrong store" header    |
 *  | 2+      | matches   | render picker (multi-tenant primary path)|
 *  | 2+      | mismatch  | render picker w/ "wrong store" header    |
 *
 * The "render picker w/ wrong store" rows are what stops the
 * /dashboard ↔ /pick-tenant loop: we never redirect to /dashboard on
 * a host whose slug isn't in the user's tenant list, because that
 * /dashboard request would just bounce right back to /pick-tenant.
 */
export default async function PickTenantPage() {
  const h = await headers();
  const uid = h.get("x-session-user-id") ?? "";
  if (!uid) {
    redirect("/login");
  }

  const [tenants, hostTenantId] = await Promise.all([
    listMemberTenants(uid).catch(() => []),
    tenantIdForHostSlug(h.get("host")),
  ]);

  const decision = decidePickTenantOutcome({ tenants, hostTenantId });
  if (decision.kind === "redirect_login") redirect("/login");
  if (decision.kind === "redirect_dashboard") redirect("/dashboard");
  const wrongStore = decision.wrongStore;

  return (
    <>
      <BrandBar />
      <main id="main" className="px-6 py-16 sm:py-24">
        <div className="mx-auto w-full max-w-xl space-y-10">
          <header className="space-y-2">
            <p className="eyebrow">{wrongStore ? "No access" : "Store access"}</p>
            <h1 className="font-serif text-4xl font-medium tracking-tight text-foreground">
              {wrongStore ? "You don't have access to this store" : "Pick a store"}
            </h1>
            <p className="text-base leading-7 text-foreground-secondary">
              {wrongStore
                ? "This subdomain belongs to a store you're not a member of. Choose one of your stores to continue, or sign out and use a different account."
                : "Your account belongs to more than one store. Choose the one you want to open and we'll switch the admin session for you."}
            </p>
          </header>
          <PickTenantForm tenants={tenants} />
        </div>
      </main>
    </>
  );
}
