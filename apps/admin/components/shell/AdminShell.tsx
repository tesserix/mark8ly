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

import { RoleBadge } from "@repo/ui/role-badge";

import type { Membership, TenantRole } from "@/lib/api/platform-api";
import { UserMenu } from "./UserMenu";
import { TenantSwitcher } from "./TenantSwitcher";

/**
 * AdminShell wraps every authenticated admin page with the sidebar +
 * topbar chrome. Paper · Ink · Moss editorial — solid background, no
 * glassmorphism, hairline rules, serif headings, ≥44px touch targets.
 */
interface AdminShellProps {
  children: ReactNode;
  tenantName?: string;
  userEmail?: string;
  role?: TenantRole;
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
  href?: string;
  children?: NavLeaf[];
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
      { label: "Storefront", href: "/settings/storefront" },
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

export function AdminShell(props: AdminShellProps) {
  return (
    <SidebarProvider>
      <AdminShellFrame {...props} />
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
        className="border-r border-border-subtle bg-background-elevated text-foreground"
      >
        <SidebarHeader className="border-b border-border-subtle px-4 py-5 group-data-[collapsible=icon]:px-2 group-data-[collapsible=icon]:py-3">
          <Link
            href="/dashboard"
            className="flex items-center gap-3 px-1 py-1 transition-opacity hover:opacity-80 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:px-0"
          >
            <span className="font-serif text-2xl font-medium tracking-[-0.025em] text-foreground group-data-[collapsible=icon]:hidden">
              mark8ly
            </span>
            <span
              aria-hidden="true"
              className="hidden font-serif text-2xl font-medium tracking-[-0.025em] text-foreground group-data-[collapsible=icon]:inline"
            >
              m
            </span>
          </Link>
          {tenantName && (
            <p className="mt-2 truncate text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary group-data-[collapsible=icon]:hidden">
              {storeLabel}
            </p>
          )}
          {memberships && memberships.length > 1 && currentTenantId && (
            <div className="mt-4 group-data-[collapsible=icon]:hidden">
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
            <div className="px-3 pb-3 pt-4 text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary group-data-[collapsible=icon]:hidden">
              Navigation
            </div>
            <SidebarGroupContent>
              <SidebarMenu className="space-y-1">
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

        <SidebarFooter className="border-t border-border-subtle p-3 group-data-[collapsible=icon]:px-2 group-data-[collapsible=icon]:py-3">
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton asChild tooltip="Sign out">
                <a
                  href="/logout"
                  className="flex items-center gap-3 text-foreground-secondary hover:text-foreground"
                >
                  <LogOut className="h-4 w-4" aria-hidden="true" />
                  <span>Sign out</span>
                </a>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
      </Sidebar>

      <SidebarInset className="relative bg-background">
        <header className="sticky top-0 z-30 border-b border-border-subtle bg-background">
          <button
            type="button"
            onClick={toggleSidebar}
            aria-label={isCollapsed ? "Expand sidebar" : "Collapse sidebar"}
            className="absolute left-0 top-1/2 z-40 hidden h-11 w-6 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full border border-border-subtle bg-background-elevated text-foreground-secondary hover:text-foreground md:inline-flex"
          >
            {isCollapsed ? (
              <ChevronRight className="h-4 w-4" aria-hidden="true" />
            ) : (
              <ChevronLeft className="h-4 w-4" aria-hidden="true" />
            )}
          </button>
          <div className="flex h-[4.5rem] items-center justify-between px-5 sm:px-7">
            <div className="flex items-center gap-3">
              <SidebarTrigger className="-ml-1 md:hidden" />
              <div className="hidden sm:block">
                <p className="eyebrow">{pageTitle.eyebrow}</p>
                <h1 className="mt-1 font-serif text-2xl font-medium leading-none tracking-tight text-foreground">
                  {pageTitle.title}
                </h1>
              </div>
            </div>

            <div className="flex items-center gap-2 sm:gap-3">
              {role && (
                <div
                  data-testid="role-badge"
                  className="hidden sm:inline-flex"
                >
                  <RoleBadge role={role} size="md" />
                </div>
              )}
              <button
                type="button"
                className="hidden h-11 w-11 items-center justify-center rounded-md text-foreground-secondary transition-colors hover:bg-paper-100 hover:text-foreground sm:inline-flex"
                aria-label="Notifications"
              >
                <Bell className="h-4 w-4" aria-hidden="true" />
              </button>
              <UserMenu email={userEmail} />
            </div>
          </div>
        </header>

        <main id="main" className="flex-1 px-4 py-8 sm:px-6 sm:py-10 lg:px-8">
          {children}
        </main>
      </SidebarInset>
    </>
  );
}

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

  if (section.href) {
    const active = activeSectionKey === section.key;
    return (
      <SidebarMenuItem>
        <SidebarMenuButton asChild isActive={active} tooltip={section.label}>
          <Link href={section.href} className="gap-3">
            <Icon className="h-4 w-4" aria-hidden="true" />
            <span>{section.label}</span>
          </Link>
        </SidebarMenuButton>
      </SidebarMenuItem>
    );
  }

  const hasActiveChild = activeSectionKey === section.key;
  const isOpen = openSectionKey === section.key;

  if (isCollapsed) {
    return (
      <DropdownMenu modal={false}>
        <SidebarMenuItem>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton tooltip={section.label} isActive={hasActiveChild}>
              <Icon className="h-4 w-4" aria-hidden="true" />
              <span>{section.label}</span>
            </SidebarMenuButton>
          </DropdownMenuTrigger>
        </SidebarMenuItem>
        <DropdownMenuContent
          side="right"
          align="start"
          sideOffset={12}
          className="min-w-[15rem] border-border-subtle bg-background-elevated p-1.5"
        >
          <DropdownMenuLabel className="px-3 py-2 text-[11px] uppercase tracking-[0.16em] text-foreground-tertiary">
            {section.label}
          </DropdownMenuLabel>
          <DropdownMenuSeparator className="mx-1 mb-1 bg-border-subtle" />
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
                    ? "bg-primary text-primary-foreground focus:bg-primary focus:text-primary-foreground"
                    : "text-foreground focus:bg-paper-100 focus:text-foreground"
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
      className="group/collapsible space-y-1"
    >
      <SidebarMenuItem>
        <CollapsibleTrigger asChild>
          <SidebarMenuButton tooltip={section.label} isActive={hasActiveChild}>
            <Icon className="h-4 w-4" aria-hidden="true" />
            <span>{section.label}</span>
            <ChevronDown
              className="ml-auto h-4 w-4 transition-transform duration-200 -rotate-90 group-data-[state=open]/collapsible:rotate-0"
              aria-hidden="true"
            />
          </SidebarMenuButton>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <SidebarMenuSub className="ml-5 border-l border-border-subtle pl-3">
            {section.children?.map((child) => (
              <SidebarMenuSubItem key={`${section.key}-${child.label}`}>
                <SidebarMenuSubButton
                  asChild
                  isActive={isChildActive(
                    pathname,
                    section.key,
                    section.children ?? [],
                    child,
                  )}
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
  const duplicateHrefCount = siblings.filter(
    (item) => item.href === child.href,
  ).length;
  if (duplicateHrefCount > 1) {
    return (
      canonicalChildLabelBySection[sectionKey] === child.label &&
      pathname === child.href
    );
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

function getPageTitle(pathname: string | null): {
  eyebrow: string;
  title: string;
} {
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
  if (pathname.startsWith("/settings/storefront")) {
    return { eyebrow: "Storefront", title: "Theme & Layout" };
  }
  if (pathname.startsWith("/settings")) {
    return { eyebrow: "Configuration", title: "Settings" };
  }
  return { eyebrow: "Workspace", title: "Admin" };
}
