import type { ReactNode } from "react";
import { cookies, headers } from "next/headers";
import { AccountSidebar } from "@/components/AccountSidebar";
import { AccountMobileNav } from "@/components/AccountMobileNav";
import { StorefrontNav } from "@/components/StorefrontNav";
import { decodeSession } from "@/lib/auth";
import { resolveStoreSlug } from "@/lib/slug";
import { enrollCustomer } from "@/lib/api/loyalty";
import { AccountSupportPanel } from "./AccountSupportPanel";

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
  const session = sessionCookie ? decodeSession(sessionCookie) : null;

  const h = await headers();
  const storeSlug = await resolveStoreSlug(h.get("host"));

  if (session) {
    const referralCode = cookieStore.get("mp_referral")?.value;
    await enrollCustomer(
      storeSlug,
      STOREFRONT_KEY,
      session.email,
      undefined,
      referralCode,
    );
  }

  const supportProps = session
    ? {
        storeSlug,
        customerEmail: session.email,
        customerName: session.email,
      }
    : null;

  return (
    <div className="min-h-screen bg-[color:var(--storefront-background,var(--paper-200))]">
      <StorefrontNav />
      <div className="mx-auto max-w-4xl px-4 py-12">
        {/* Mobile nav - horizontal scroll */}
        <AccountMobileNav />

        {/* Header-slot support panel — visible above every /account/* page
            for signed-in customers so reaching out never requires
            navigating away. */}
        {supportProps && (
          <div className="mb-8">
            <AccountSupportPanel {...supportProps} variant="header" />
          </div>
        )}

        <div className="flex gap-8">
          <aside className="hidden w-48 shrink-0 md:block">
            <AccountSidebar />
          </aside>
          <main className="min-w-0 flex-1">{children}</main>
        </div>

        {/* Footer-slot support panel — same form, second placement so
            shoppers who scroll to the bottom of a long page still have
            a handoff point without scrolling back up. */}
        {supportProps && (
          <div className="mt-12">
            <AccountSupportPanel {...supportProps} variant="footer" />
          </div>
        )}
      </div>
    </div>
  );
}
