"use client";

import type { ReactNode } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  BarChart3,
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
interface AdminShellProps {
  children: ReactNode;
  tenantName?: string;
  userEmail?: string;
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
}: AdminShellProps) {
  return (
    <SidebarProvider>
      <Sidebar collapsible="icon" className="border-r border-border">
        <SidebarHeader className="border-b border-border px-4 py-4">
          <Link
            href="/dashboard"
            className="flex items-center gap-3 hover:opacity-80 transition-opacity"
          >
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src="/icon-192.png"
              alt="mark8ly"
              className="h-8 w-8 shrink-0 rounded-lg"
            />
            <div className="min-w-0 group-data-[collapsible=icon]:hidden">
              <p className="truncate font-serif text-lg font-medium text-foreground">
                {tenantName ?? "mark8ly"}
              </p>
              <p className="truncate text-xs text-muted-foreground">
                Admin console
              </p>
            </div>
          </Link>
        </SidebarHeader>

        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupContent>
              <SidebarMenu>
                {navigation.map((section) => (
                  <NavSectionItem key={section.key} section={section} />
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>

        <SidebarFooter className="border-t border-border p-3">
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton asChild tooltip="Sign out">
                <a
                  href="/logout"
                  className="flex items-center gap-3 text-muted-foreground hover:text-foreground"
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
        <header className="sticky top-0 z-30 flex h-16 items-center justify-between border-b border-border bg-background/82 px-6 backdrop-blur-sm">
          <div className="flex items-center gap-3">
            <SidebarTrigger className="-ml-1" />
            <div className="hidden sm:block">
              {tenantName && (
                <p className="text-xs uppercase tracking-[0.14em] text-muted-foreground">
                  {tenantName}
                </p>
              )}
            </div>
          </div>

          <div className="flex items-center gap-3">
            <UserMenu email={userEmail} />
          </div>
        </header>

        <main className="flex-1 px-6 py-10 sm:px-10 sm:py-12">{children}</main>
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
function NavSectionItem({ section }: { section: NavSection }) {
  const pathname = usePathname();
  const Icon = section.icon;

  // Simple link item.
  if (section.href) {
    const active = isActiveLink(pathname, section.href);
    return (
      <SidebarMenuItem>
        <SidebarMenuButton asChild isActive={active} tooltip={section.label}>
          <Link href={section.href}>
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

  return (
    <Collapsible defaultOpen={hasActiveChild} className="group/collapsible">
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
