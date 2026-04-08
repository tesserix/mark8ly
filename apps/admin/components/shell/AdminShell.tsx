"use client";

import { useEffect, useState, type ReactNode } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  BarChart3,
  Bell,
  ChevronRight,
  Package,
  ShoppingCart,
  Users,
  Megaphone,
  Settings,
  HelpCircle,
  LogOut,
  ChevronDown,
} from "lucide-react";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  SidebarProvider,
  SidebarTrigger,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@tesserix/web";

import { UserMenu } from "./UserMenu";

/**
 * AdminShell wraps every authenticated admin page with the sidebar +
 * topbar chrome. The structure is ported from
 * mark8ly_backup/apps/admin/app/(tenant)/layout.tsx — same section
 * groupings, same icons, same collapsible sub-nav. Role filtering and
 * the tenant switcher are out of scope for Phase K and will land
 * alongside the real RBAC work.
 *
 * Navigation items whose destinations don't exist yet (catalog,
 * orders, customers, marketing children) render as collapsed groups
 * that link to the corresponding /products, /orders, /customers,
 * /settings stub routes. Everything under the shell is an
 * authenticated route; middleware has already gated the request.
 */
import type { TenantRole } from "@/lib/api/platform-api";

interface AdminShellProps {
  children: ReactNode;
  tenantName?: string;
  userEmail?: string;
  // Phase O: forwarded so the shell can surface a role badge in the
  // top bar and (in the future) filter sub-nav items by permission.
  // Phase O ships with all four roles able to see all nav items;
  // role-gated sub-nav lands with invite-teammate.
  role?: TenantRole;
}

interface NavLeaf {
  label: string;
  href: string;
}

interface NavSection {
  key: string;
  label: string;
  icon: typeof BarChart3;
  href?: string; // top-level link (no children)
  children?: NavLeaf[]; // collapsible group
}

const navigation: NavSection[] = [
  {
    key: "analytics",
    label: "Analytics",
    icon: BarChart3,
    children: [
      { label: "Overview", href: "/dashboard" },
      { label: "Sales", href: "/dashboard" },
      { label: "Customers", href: "/dashboard" },
      { label: "Inventory", href: "/dashboard" },
    ],
  },
  {
    key: "catalog",
    label: "Catalog",
    icon: Package,
    children: [
      { label: "Products", href: "/products" },
      { label: "Categories", href: "/products" },
      { label: "Inventory", href: "/products" },
    ],
  },
  {
    key: "orders",
    label: "Orders",
    icon: ShoppingCart,
    children: [
      { label: "All Orders", href: "/orders" },
      { label: "Returns & Refunds", href: "/orders" },
      { label: "Abandoned Carts", href: "/orders" },
    ],
  },
  {
    key: "customers",
    label: "Customers",
    icon: Users,
    children: [
      { label: "All Customers", href: "/customers" },
      { label: "Reviews", href: "/customers" },
    ],
  },
  {
    key: "marketing",
    label: "Marketing",
    icon: Megaphone,
    children: [
      { label: "Campaigns", href: "/dashboard" },
      { label: "Coupons", href: "/dashboard" },
      { label: "Gift Cards", href: "/dashboard" },
      { label: "Loyalty", href: "/dashboard" },
    ],
  },
  {
    key: "settings",
    label: "Settings",
    icon: Settings,
    children: [
      { label: "Store Settings", href: "/settings" },
      { label: "Shipping", href: "/settings" },
      { label: "Payments", href: "/settings" },
      { label: "Legal", href: "/settings" },
    ],
  },
  {
    key: "support",
    label: "Support",
    icon: HelpCircle,
    href: "/dashboard",
  },
];

export function AdminShell({
  children,
  tenantName,
  userEmail,
  role,
}: AdminShellProps) {
  const pathname = usePathname();
  const storeLabel = tenantName ?? "mark8ly";
  const pageTitle = getPageTitle(pathname);
  const activeSectionKey =
    navigation.find((section) =>
      section.href
        ? isActiveLink(pathname, section.href)
        : section.children?.some((child) => isActiveLink(pathname, child.href)),
    )?.key ?? null;
  const [openSectionKey, setOpenSectionKey] = useState<string | null>(
    activeSectionKey,
  );

  useEffect(() => {
    if (activeSectionKey) {
      setOpenSectionKey(activeSectionKey);
    }
  }, [activeSectionKey]);

  return (
    <SidebarProvider>
      <Sidebar
        collapsible="icon"
        className="border-r border-sidebar-border/70 bg-[linear-gradient(180deg,rgba(34,28,23,0.985),rgba(27,22,19,0.97))] text-sidebar-foreground"
      >
        <SidebarHeader className="border-b border-sidebar-border/70 px-4 py-4">
          <Link
            href="/dashboard"
            className="flex items-center gap-3 rounded-[1.35rem] px-2 py-2 transition-opacity hover:opacity-90"
          >
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src="/icon-192.png"
              alt="mark8ly"
              className="h-10 w-10 shrink-0 rounded-[1.15rem] border border-white/10 shadow-[0_14px_34px_rgba(0,0,0,0.28)]"
            />
            <div className="min-w-0 group-data-[collapsible=icon]:hidden">
              <p className="truncate font-serif text-xl font-medium tracking-tight text-sidebar-foreground">
                mark8ly
              </p>
              <p className="truncate text-xs uppercase tracking-[0.18em] text-sidebar-text-muted">
                {storeLabel}
              </p>
            </div>
          </Link>
        </SidebarHeader>

        <SidebarContent className="sidebar-scrollbar">
          <div className="px-4 py-4 group-data-[collapsible=icon]:hidden">
            <div className="rounded-[1.35rem] border border-white/8 bg-[linear-gradient(180deg,rgba(255,255,255,0.06),rgba(255,255,255,0.03))] px-4 py-4 shadow-[inset_0_1px_0_rgba(255,255,255,0.05)]">
              <p className="text-[11px] font-semibold uppercase tracking-[0.18em] text-sidebar-text-muted">
                Workspace
              </p>
              <p className="mt-2 text-sm font-medium text-sidebar-foreground">
                {storeLabel}
              </p>
              <p className="mt-1 text-sm leading-6 text-sidebar-text">
                Products, orders, and settings all plug into this shell as the admin grows.
              </p>
            </div>
          </div>

          <SidebarGroup className="px-2">
            <div className="px-4 pb-2 pt-1 text-[11px] font-semibold uppercase tracking-[0.18em] text-sidebar-text-muted group-data-[collapsible=icon]:hidden">
              Navigation
            </div>
            <SidebarGroupContent>
              <SidebarMenu>
                {navigation.map((section) => (
                  <NavSectionItem
                    key={section.key}
                    section={section}
                    openSectionKey={openSectionKey}
                    onToggleSection={setOpenSectionKey}
                  />
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>

        <SidebarFooter className="border-t border-sidebar-border/70 p-3">
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton asChild tooltip="Sign out">
                <a
                  href="/logout"
                  className="flex items-center gap-3 text-sidebar-text hover:text-sidebar-foreground"
                >
                  <LogOut className="h-4 w-4" />
                  <span>Sign out</span>
                </a>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
      </Sidebar>

      <SidebarInset>
        <header className="sticky top-0 z-30 border-b admin-soft-rule bg-[rgba(248,243,236,0.72)] backdrop-blur-xl">
          <div className="flex h-[4.5rem] items-center justify-between px-5 py-4 sm:px-7">
            <div className="flex items-center gap-3">
              <SidebarTrigger className="-ml-1" />
              <div className="hidden sm:block">
                <p className="admin-eyebrow">{pageTitle.eyebrow}</p>
                <div className="mt-1 flex items-center gap-2">
                  <span className="font-serif text-[1.45rem] leading-none tracking-tight text-foreground">
                    {pageTitle.title}
                  </span>
                  <span className="rounded-full border border-border/80 bg-white/70 px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--app-brown)]">
                    {storeLabel}
                  </span>
                </div>
              </div>
            </div>

            <div className="flex items-center gap-2 sm:gap-3">
              {role && (
                <span
                  data-testid="role-badge"
                  className="hidden rounded-full border border-border/70 bg-white/78 px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.14em] text-muted-foreground sm:inline-flex"
                >
                  {role}
                </span>
              )}
              <button
                type="button"
                className="hidden h-10 w-10 items-center justify-center rounded-full border border-border/80 bg-white/70 text-muted-foreground transition-colors hover:text-foreground sm:inline-flex"
                aria-label="Notifications"
              >
                <Bell className="h-4 w-4" />
              </button>
              <UserMenu email={userEmail} />
            </div>
          </div>
        </header>

        <main className="flex-1 px-4 py-6 sm:px-6 sm:py-8 lg:px-8">{children}</main>
      </SidebarInset>
    </SidebarProvider>
  );
}

/**
 * Renders a single navigation section. A section with children is a
 * collapsible group that expands when the current pathname matches
 * any of its descendants; a section with a direct `href` is a simple
 * link.
 */
function NavSectionItem({
  section,
  openSectionKey,
  onToggleSection,
}: {
  section: NavSection;
  openSectionKey: string | null;
  onToggleSection: (key: string | null) => void;
}) {
  const pathname = usePathname();
  const Icon = section.icon;

  // Simple link item.
  if (section.href) {
    const active = isActiveLink(pathname, section.href);
    return (
      <SidebarMenuItem>
        <SidebarMenuButton asChild isActive={active} tooltip={section.label}>
          <Link href={section.href} className="gap-3">
            <Icon className="h-4 w-4" />
            <span>{section.label}</span>
          </Link>
        </SidebarMenuButton>
      </SidebarMenuItem>
    );
  }

  // Collapsible group.
  const hasActiveChild =
    section.children?.some((c) => isActiveLink(pathname, c.href)) ?? false;
  const isOpen = openSectionKey === section.key;

  return (
    <Collapsible
      open={isOpen}
      onOpenChange={(open) => onToggleSection(open ? section.key : null)}
      className="group/collapsible"
    >
      <SidebarMenuItem>
        <CollapsibleTrigger asChild>
          <SidebarMenuButton tooltip={section.label} isActive={hasActiveChild}>
            <Icon className="h-4 w-4" />
            <span>{section.label}</span>
            <ChevronDown className="ml-auto h-4 w-4 transition-transform duration-200 group-data-[state=open]/collapsible:rotate-0 -rotate-90" />
          </SidebarMenuButton>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <SidebarMenuSub>
            {section.children?.map((child) => (
              <SidebarMenuSubItem key={`${section.key}-${child.label}`}>
                <SidebarMenuSubButton
                  asChild
                  isActive={isActiveLink(pathname, child.href)}
                >
                  <Link href={child.href}>{child.label}</Link>
                </SidebarMenuSubButton>
              </SidebarMenuSubItem>
            ))}
          </SidebarMenuSub>
        </CollapsibleContent>
      </SidebarMenuItem>
    </Collapsible>
  );
}

function isActiveLink(pathname: string | null, href: string): boolean {
  if (!pathname) return false;
  if (pathname === href) return true;
  return pathname.startsWith(`${href}/`);
}

function getPageTitle(pathname: string | null): { eyebrow: string; title: string } {
  if (!pathname || pathname === "/" || pathname === "/dashboard") {
    return { eyebrow: "Overview", title: "Dashboard" };
  }

  if (pathname.startsWith("/products")) {
    return { eyebrow: "Catalog", title: "Products" };
  }

  if (pathname.startsWith("/orders")) {
    return { eyebrow: "Operations", title: "Orders" };
  }

  if (pathname.startsWith("/customers")) {
    return { eyebrow: "Relationships", title: "Customers" };
  }

  if (pathname.startsWith("/settings/general")) {
    return { eyebrow: "Store Setup", title: "General Settings" };
  }

  if (pathname.startsWith("/settings")) {
    return { eyebrow: "Configuration", title: "Settings" };
  }

  return { eyebrow: "Workspace", title: "Admin" };
}
