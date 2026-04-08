"use client";

import { useEffect, useState, type ReactNode } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  BarChart3,
  Bell,
  ChevronLeft,
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
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  useSidebar,
} from "@tesserix/web";

import { UserMenu } from "./UserMenu";

/**
 * AdminShell wraps every authenticated admin page with the sidebar +
 * topbar chrome. The structure is ported from
 * mark8ly_backup/apps/admin/app/(tenant)/layout.tsx — same section
 * groupings and same icons, with the multi-store switcher promoted
 * into the shell so changing tenants feels like a core workspace
 * action instead of a detour.
 *
 * Navigation items whose destinations don't exist yet (catalog,
 * orders, customers, marketing children) render as collapsed groups
 * that link to the corresponding /products, /orders, /customers,
 * /settings stub routes. Everything under the shell is an
 * authenticated route; middleware has already gated the request.
 */
import type { Membership, TenantRole } from "@/lib/api/platform-api";
import { TenantSwitcher } from "./TenantSwitcher";

interface AdminShellProps {
  children: ReactNode;
  tenantName?: string;
  userEmail?: string;
  // Phase O: forwarded so the shell can surface a role badge in the
  // top bar and (in the future) filter sub-nav items by permission.
  role?: TenantRole;
  // Phase P: every tenant the signed-in user belongs to. When the
  // list has more than one entry the sidebar renders a switcher so
  // users can move between stores without leaving admin. Pre-fetched
  // via getServerSessionContext() on every page that uses the shell.
  memberships?: Membership[];
  currentTenantId?: string;
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
      { label: "Store Settings", href: "/settings/general" },
      { label: "Stores", href: "/settings/stores" },
      { label: "Team", href: "/settings/team" },
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
  memberships,
  currentTenantId,
}: AdminShellProps) {
  return (
    <SidebarProvider>
      <AdminShellFrame
        children={children}
        tenantName={tenantName}
        userEmail={userEmail}
        role={role}
        memberships={memberships}
        currentTenantId={currentTenantId}
      />
    </SidebarProvider>
  );
}

function AdminShellFrame({
  children,
  tenantName,
  userEmail,
  role,
  memberships,
  currentTenantId,
}: AdminShellProps) {
  const pathname = usePathname();
  const { state, toggleSidebar } = useSidebar();
  const storeLabel = tenantName ?? "mark8ly";
  const pageTitle = getPageTitle(pathname);
  const activeSectionKey = getActiveSectionKey(pathname);
  const [openSectionKey, setOpenSectionKey] = useState<string | null>(
    activeSectionKey,
  );
  const isCollapsed = state === "collapsed";

  useEffect(() => {
    if (activeSectionKey) {
      setOpenSectionKey(activeSectionKey);
    }
  }, [activeSectionKey]);

  return (
    <>
      <Sidebar
        collapsible="icon"
        className="border-r border-sidebar-border/70 bg-[linear-gradient(180deg,rgba(34,28,23,0.985),rgba(27,22,19,0.97))] text-sidebar-foreground"
      >
        <SidebarHeader className="border-b border-sidebar-border/70 px-4 py-4 group-data-[collapsible=icon]:px-2 group-data-[collapsible=icon]:py-3">
          <Link
            href="/dashboard"
            className="flex items-center gap-3 rounded-[1.35rem] px-2 py-2 transition-opacity hover:opacity-90 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
          >
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src="/icon-192.png"
              alt="mark8ly"
              className="h-10 w-10 shrink-0 rounded-[1.15rem] border border-white/10 shadow-[0_14px_34px_rgba(0,0,0,0.28)] group-data-[collapsible=icon]:h-9 group-data-[collapsible=icon]:w-9"
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
          {memberships && memberships.length > 1 && currentTenantId && (
            <div className="mt-4 px-2 group-data-[collapsible=icon]:hidden">
              <TenantSwitcher
                memberships={memberships}
                currentTenantId={currentTenantId}
                label="Switch store"
              />
            </div>
          )}
        </SidebarHeader>

        <SidebarContent className="sidebar-scrollbar">
          <SidebarGroup className="px-2 group-data-[collapsible=icon]:px-1.5">
            <div className="px-4 pb-3 pt-3 text-[11px] font-semibold uppercase tracking-[0.18em] text-sidebar-text-muted group-data-[collapsible=icon]:hidden">
              Navigation
            </div>
            <SidebarGroupContent>
              <SidebarMenu className="space-y-1.5">
                {navigation.map((section) => (
                  <NavSectionItem
                    key={section.key}
                    section={section}
                    activeSectionKey={activeSectionKey}
                    openSectionKey={openSectionKey}
                    onToggleSection={setOpenSectionKey}
                  />
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>

        <SidebarFooter className="border-t border-sidebar-border/70 p-3 group-data-[collapsible=icon]:px-2 group-data-[collapsible=icon]:py-3">
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

      <SidebarInset className="relative">
        <header className="sticky top-0 z-30 border-b admin-soft-rule bg-[rgba(248,243,236,0.72)] backdrop-blur-xl">
          <button
            type="button"
            onClick={toggleSidebar}
            aria-label={isCollapsed ? "Expand sidebar" : "Collapse sidebar"}
            className="absolute left-0 top-1/2 z-40 hidden h-12 w-7 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full border border-[rgba(221,206,188,0.95)] bg-[rgba(251,246,238,0.98)] text-[var(--app-brown)] shadow-[0_10px_24px_rgba(76,52,24,0.12)] transition-[transform,box-shadow,background-color,color] hover:-translate-x-[52%] hover:-translate-y-1/2 hover:bg-[rgba(255,250,244,1)] hover:shadow-[0_14px_30px_rgba(76,52,24,0.16)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[rgba(138,100,64,0.18)] md:inline-flex"
          >
            {isCollapsed ? (
              <ChevronRight className="h-4 w-4" />
            ) : (
              <ChevronLeft className="h-4 w-4" />
            )}
          </button>
          <div className="flex h-[4.5rem] items-center justify-between px-5 py-4 sm:px-7">
            <div className="flex items-center gap-3">
              <SidebarTrigger className="-ml-1 md:hidden" />
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
    </>
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
  activeSectionKey,
  openSectionKey,
  onToggleSection,
}: {
  section: NavSection;
  activeSectionKey: string | null;
  openSectionKey: string | null;
  onToggleSection: (key: string | null) => void;
}) {
  const pathname = usePathname();
  const { state } = useSidebar();
  const Icon = section.icon;
  const isCollapsed = state === "collapsed";

  // Simple link item.
  if (section.href) {
    const active = activeSectionKey === section.key;
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
  const hasActiveChild = activeSectionKey === section.key;
  const isOpen = openSectionKey === section.key;

  if (isCollapsed) {
    return (
      <DropdownMenu modal={false}>
        <SidebarMenuItem>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton tooltip={section.label} isActive={hasActiveChild}>
              <Icon className="h-4 w-4" />
              <span>{section.label}</span>
            </SidebarMenuButton>
          </DropdownMenuTrigger>
        </SidebarMenuItem>
        <DropdownMenuContent
          side="right"
          align="start"
          sideOffset={12}
          className="min-w-[15rem] rounded-[1.25rem] border-border/80 bg-[rgba(255,252,248,0.98)] p-1.5 shadow-[0_24px_60px_rgba(76,52,24,0.14)]"
        >
          <DropdownMenuLabel className="px-3 py-2 text-[11px] uppercase tracking-[0.18em] text-[var(--app-brown)]">
            {section.label}
          </DropdownMenuLabel>
          <DropdownMenuSeparator className="mx-1 mb-1 bg-border/70" />
          {section.children?.map((child) => {
            const active = isChildActive(
              pathname,
              section.key,
              section.children ?? [],
              child,
            );

            return (
              <DropdownMenuItem
                key={`${section.key}-${child.label}`}
                asChild
                className={
                  active
                    ? "rounded-xl bg-primary text-primary-foreground focus:bg-primary focus:text-primary-foreground"
                    : "rounded-xl text-foreground focus:bg-[rgba(138,100,64,0.12)] focus:text-foreground"
                }
              >
                <Link href={child.href}>{child.label}</Link>
              </DropdownMenuItem>
            );
          })}
        </DropdownMenuContent>
      </DropdownMenu>
    );
  }

  return (
    <Collapsible
      open={isOpen}
      onOpenChange={(open) => onToggleSection(open ? section.key : null)}
      className="group/collapsible space-y-1.5"
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
          <SidebarMenuSub className="ml-5 border-l border-white/10 pl-3">
            {section.children?.map((child) => (
              <SidebarMenuSubItem key={`${section.key}-${child.label}`}>
                <SidebarMenuSubButton
                  asChild
                  isActive={isChildActive(pathname, section.key, section.children ?? [], child)}
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

function getActiveSectionKey(pathname: string | null): string | null {
  if (!pathname || pathname === "/dashboard") {
    return null;
  }

  return (
    navigation.find((section) =>
      section.href
        ? isActiveLink(pathname, section.href)
        : section.children?.some((child) => isActiveLink(pathname, child.href)),
    )?.key ?? null
  );
}

function isChildActive(
  pathname: string | null,
  sectionKey: string,
  siblings: NavLeaf[],
  child: NavLeaf,
): boolean {
  if (!pathname) return false;

  const duplicateHrefCount = siblings.filter((item) => item.href === child.href).length;
  if (duplicateHrefCount > 1) {
    return canonicalChildLabelBySection[sectionKey] === child.label && pathname === child.href;
  }

  return isActiveLink(pathname, child.href);
}

const canonicalChildLabelBySection: Record<string, string> = {
  analytics: "Overview",
  catalog: "Products",
  orders: "All Orders",
  customers: "All Customers",
  marketing: "Campaigns",
  settings: "Store Settings",
};

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
