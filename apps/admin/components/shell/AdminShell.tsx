import type { ReactNode } from "react";
import Link from "next/link";
import {
  LayoutDashboard,
  Package,
  ShoppingBag,
  Users,
  Settings,
  LogOut,
} from "lucide-react";

interface AdminShellProps {
  children: ReactNode;
  tenantName?: string;
}

interface NavItem {
  href: string;
  label: string;
  icon: typeof LayoutDashboard;
}

const navItems: NavItem[] = [
  { href: "/dashboard", label: "Dashboard", icon: LayoutDashboard },
  { href: "/products", label: "Products", icon: Package },
  { href: "/orders", label: "Orders", icon: ShoppingBag },
  { href: "/customers", label: "Customers", icon: Users },
  { href: "/settings", label: "Settings", icon: Settings },
];

/**
 * AdminShell wraps every authenticated admin page with the sidebar +
 * header chrome. The sidebar nav items are the five top-level
 * categories in the legacy admin. For the Phase I "auth + blank pages"
 * slice every non-dashboard route renders a <ComingSoon /> placeholder
 * under this shell.
 */
export function AdminShell({ children, tenantName }: AdminShellProps) {
  return (
    <div className="flex min-h-screen bg-background text-foreground">
      {/* Sidebar */}
      <aside
        className="hidden md:flex w-64 shrink-0 flex-col border-r border-warm-200 bg-white/82 backdrop-blur-sm"
        aria-label="Primary navigation"
      >
        <div className="flex h-20 items-center gap-3 border-b border-warm-200 px-6">
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src="/icon-192.png"
            alt="Mark8ly"
            className="h-9 w-9 rounded-xl"
          />
          <span className="font-serif text-xl font-medium text-foreground">
            mark8ly
          </span>
        </div>

        <nav className="flex-1 p-4">
          <ul className="space-y-1">
            {navItems.map((item) => (
              <li key={item.href}>
                <Link
                  href={item.href}
                  className="flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium text-foreground-secondary transition-colors hover:bg-warm-50 hover:text-foreground"
                >
                  <item.icon className="h-4 w-4" />
                  {item.label}
                </Link>
              </li>
            ))}
          </ul>
        </nav>

        <div className="border-t border-warm-200 p-4">
          <a
            href="/logout"
            className="flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium text-foreground-secondary transition-colors hover:bg-warm-50 hover:text-foreground"
          >
            <LogOut className="h-4 w-4" />
            Sign out
          </a>
        </div>
      </aside>

      {/* Main area */}
      <div className="flex-1 flex flex-col min-w-0">
        <header className="sticky top-0 z-30 flex h-16 items-center justify-between border-b border-warm-200 bg-background/82 px-8 backdrop-blur-sm">
          <div>
            {tenantName && (
              <p className="text-xs uppercase tracking-[0.14em] text-foreground-tertiary">
                {tenantName}
              </p>
            )}
          </div>
          <div className="flex items-center gap-3">
            <a
              href="/settings"
              className="inline-flex h-9 w-9 items-center justify-center rounded-full border border-warm-200 bg-white text-foreground-secondary transition-colors hover:border-warm-300 hover:text-foreground"
              aria-label="Settings"
            >
              <Settings className="h-4 w-4" />
            </a>
          </div>
        </header>

        <main className="flex-1 px-6 py-10 sm:px-10 sm:py-12">{children}</main>
      </div>
    </div>
  );
}
