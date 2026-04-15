import type { ReactNode } from "react";
import { cookies, headers } from "next/headers";
import { AccountSidebar } from "@/components/AccountSidebar";
import { AccountMobileNav } from "@/components/AccountMobileNav";
import { StorefrontNav } from "@/components/StorefrontNav";
import { decodeSession } from "@/lib/auth";
import { slugFromHost } from "@/lib/slug";
import { enrollCustomer } from "@/lib/api/loyalty";

const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";

interface AccountLayoutProps {
  children: ReactNode;
}

export default async function AccountLayout({ children }: AccountLayoutProps) {
  // Auto-enroll on any /account/* visit. Backend enroll is idempotent —
  // returns the existing record for already-enrolled customers — so this
  // is a single cheap DB lookup. Catches customers who signed up before
  // the loyalty program went live and never passed through /account/loyalty.
  const cookieStore = await cookies();
  const sessionCookie = cookieStore.get("mp_customer_session")?.value;
  if (sessionCookie) {
    const session = decodeSession(sessionCookie);
    if (session) {
      const h = await headers();
      const storeSlug =
        slugFromHost(h.get("host")) ||
        process.env.DEFAULT_STORE_SLUG ||
        "default";
      const referralCode = cookieStore.get("mp_referral")?.value;
      await enrollCustomer(
        storeSlug,
        STOREFRONT_KEY,
        session.email,
        undefined,
        referralCode,
      );
    }
  }

  return (
    <div className="min-h-screen bg-[color:var(--storefront-background,var(--paper-200))]">
      <StorefrontNav />
      <div className="mx-auto max-w-4xl px-4 py-12">
        {/* Mobile nav - horizontal scroll */}
        <AccountMobileNav />

        <div className="flex gap-8">
          <aside className="hidden w-48 shrink-0 md:block">
            <AccountSidebar />
          </aside>
          <main className="min-w-0 flex-1">{children}</main>
        </div>
      </div>
    </div>
  );
}
