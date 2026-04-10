import type { ReactNode } from "react";
import Link from "next/link";
import { AccountSidebar } from "@/components/AccountSidebar";
import { AccountMobileNav } from "@/components/AccountMobileNav";

interface AccountLayoutProps {
  children: ReactNode;
}

export default function AccountLayout({ children }: AccountLayoutProps) {
  return (
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
  );
}
