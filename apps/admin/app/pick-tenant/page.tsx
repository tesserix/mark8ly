import type { Metadata } from "next";
import { headers } from "next/headers";
import { redirect } from "next/navigation";
import { BrandBar } from "@repo/ui/brand-bar";

import { listMemberTenants } from "@/lib/api/platform-api";
import { PickTenantForm } from "@/components/auth/PickTenantForm";

export const metadata: Metadata = { title: "Pick a store" };

/**
 * /pick-tenant — landing for users with more than one tenant.
 *
 * Signed-in via admin /login → multi-tenant membership detected →
 * redirected here. Solo-founder users skip straight to /dashboard;
 * if someone navigates here with zero or one tenant we redirect away.
 */
export default async function PickTenantPage() {
  const h = await headers();
  const uid = h.get("x-session-user-id") ?? "";
  if (!uid) {
    redirect("/login");
  }

  const tenants = await listMemberTenants(uid).catch(() => []);

  if (tenants.length === 0) {
    redirect("/login");
  }
  if (tenants.length === 1) {
    redirect("/dashboard");
  }

  return (
    <>
      <BrandBar />
      <main id="main" className="px-6 py-16 sm:py-24">
        <div className="mx-auto w-full max-w-xl space-y-10">
          <header className="space-y-2">
            <p className="eyebrow">Store access</p>
            <h1 className="font-serif text-4xl font-medium tracking-tight text-foreground">
              Pick a store
            </h1>
            <p className="text-base leading-7 text-foreground-secondary">
              Your account belongs to more than one store. Choose the one you
              want to open and we&apos;ll switch the admin session for you.
            </p>
          </header>
          <PickTenantForm tenants={tenants} />
        </div>
      </main>
    </>
  );
}
