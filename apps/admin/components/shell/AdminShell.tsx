"use client";

import { Fragment, useEffect, useState, type ReactNode } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  BarChart3,
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

import type { Membership, Store, TenantRole } from "@/lib/api/platform-api";
import { UserMenu } from "./UserMenu";
import { TenantSwitcher } from "./TenantSwitcher";
import { StoreSwitcher } from "./StoreSwitcher";
import { NotificationBell } from "./NotificationBell";
import { BannerStack } from "./banners/BannerStack";

/**
 * AdminShell wraps every authenticated admin page with the sidebar +
 * topbar chrome. Paper · Ink · Moss editorial — solid background, no
 * glassmorphism, hairline rules, serif headings, ≥44px touch targets.
 */
interface AdminShellProps {
  children: ReactNode;
  tenantName?: string;
  userEmail?: string;
  userName?: string;
  userAvatarUrl?: string;
  role?: TenantRole;
  memberships?: Membership[];
  currentTenantId?: string;
  stores?: Store[];
  currentStoreId?: string;
  userId?: string;
  tenantId?: string;
}

interface NavLeaf {
  label: string;
  href: string;
  /**
   * Optional group label for sections with many items. Consecutive leaves
   * sharing a group render under a single eyebrow header — used by Settings
   * to break 10+ items into 4 logical clusters (STORE / SELLING / TEAM & ACCESS
   * / ACCOUNT). Leaves without a group render ungrouped.
   */
  group?: string;
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
    key: "dashboard",
    label: "Dashboard",
    icon: BarChart3,
    href: "/dashboard",
  },
  {
    key: "catalog",
    label: "Products",
    icon: Package,
    href: "/products",
  },
  {
    key: "orders",
    label: "Orders",
    icon: ShoppingCart,
    children: [
      { label: "All Orders", href: "/orders" },
      { label: "Returns & replacements", href: "/orders/returns" },
    ],
  },
  {
    key: "customers",
    label: "Customers",
    icon: Users,
    children: [
      { label: "All Customers", href: "/customers" },
      { label: "Reviews", href: "/customers/reviews" },
    ],
  },
  {
    key: "marketing",
    label: "Marketing",
    icon: Megaphone,
    children: [
      { label: "Campaigns", href: "/marketing/campaigns" },
      { label: "Segments", href: "/marketing/segments" },
      { label: "Coupons", href: "/marketing/coupons" },
      { label: "Gift Cards", href: "/marketing/gift-cards" },
      { label: "Loyalty", href: "/marketing/loyalty" },
    ],
  },
  {
    key: "settings",
    label: "Settings",
    icon: Settings,
    children: [
      { group: "Store", label: "Stores", href: "/settings/stores" },
      // Pages have moved into the Branding → Pages tab.
      { group: "Store", label: "Branding", href: "/settings/themes" },
      { group: "Store", label: "Domains", href: "/settings/domains" },
      { group: "Selling", label: "Payments", href: "/settings/payments" },
      { group: "Selling", label: "Shipping", href: "/settings/shipping" },
      { group: "Team & access", label: "Team", href: "/settings/team" },
      { group: "Team & access", label: "Audit Logs", href: "/settings/audit-logs" },
      { group: "Account", label: "Account", href: "/settings/account" },
      { group: "Account", label: "Notifications", href: "/settings/notifications" },
      { group: "Account", label: "Billing", href: "/settings/billing" },
    ],
  },
  {
    key: "support",
    label: "Support",
    icon: HelpCircle,
    children: [
      // Otto Live chat + Audit log are temporarily hidden from the
      // nav until product launch. The routes + components remain in
      // the tree so re-enabling is a matter of uncommenting these
      // entries. See docs/otto-disabled.md for the full checklist.
      // { label: "Live chat", href: "/support/live-chat" },
      // { label: "Audit log", href: "/support/audit-log" },
      { label: "Tickets", href: "/support/tickets" },
      { label: "Help Center", href: "/support/help" },
    ],
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
  userName,
  userAvatarUrl,
  role,
  memberships,
  currentTenantId,
  stores,
  currentStoreId,
  userId,
  tenantId,
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
        role="navigation"
        aria-label="Main navigation"
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
          {/* Show the TenantSwitcher when the caller belongs to more
              than one store; otherwise fall back to a plain uppercase
              label. Never render both — the switcher already shows the
              current store name, and stacking the label above it just
              repeats the same text twice. */}
          {memberships && memberships.length > 1 && currentTenantId ? (
            <div className="mt-4 group-data-[collapsible=icon]:hidden">
              <TenantSwitcher
                memberships={memberships}
                currentTenantId={currentTenantId}
                label="Switch company"
              />
            </div>
          ) : (
            tenantName && (
              <p className="mt-2 truncate text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary group-data-[collapsible=icon]:hidden">
                {storeLabel}
              </p>
            )
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
              {/* Compact section indicator only — the page body owns
                  the real H1 via AdminPage, so rendering the same title
                  here as a second H1 was double-billing every route. */}
              <p
                className="hidden text-[11px] font-semibold uppercase tracking-[0.18em] text-foreground-tertiary sm:block"
                aria-label={`${pageTitle.eyebrow} / ${pageTitle.title}`}
              >
                {pageTitle.eyebrow}
                <span className="mx-2 text-foreground-tertiary/50" aria-hidden="true">
                  /
                </span>
                <span className="text-foreground-secondary">{pageTitle.title}</span>
              </p>
            </div>

            <div className="flex items-center gap-2 sm:gap-3">
              {stores && stores.length > 1 && currentStoreId && (
                <div className="hidden sm:inline-flex">
                  <StoreSwitcher
                    stores={stores}
                    currentStoreId={currentStoreId}
                    label="Switch store"
                    compact
                  />
                </div>
              )}
              {role && (
                <div
                  data-testid="role-badge"
                  className="hidden sm:inline-flex"
                >
                  <RoleBadge role={role} size="md" />
                </div>
              )}
              {currentStoreId && userId && tenantId && (
                <div className="hidden sm:inline-flex">
                  <NotificationBell
                    storeId={currentStoreId}
                    userId={userId}
                    tenantId={tenantId}
                  />
                </div>
              )}
              <UserMenu
                email={userEmail}
                name={userName}
                avatarUrl={userAvatarUrl}
              />
            </div>
          </div>
        </header>

        {/* Priority-selective banner slot — renders at most one banner at a
            time based on subscription state. Only mounted when storeId is
            available (store-scoped routes); hidden on the store list page
            and other non-store surfaces where currentStoreId is undefined. */}
        {currentStoreId && <BannerStack storeId={currentStoreId} />}

        {/* Single source of truth for admin page width. Every admin surface
            renders inside this max-w-5xl container; individual pages should
            NOT wrap themselves in another mx-auto/max-w container. Padding
            comes from the outer main element so the container can be pure. */}
        <main id="main" className="flex-1 px-4 py-8 sm:px-6 sm:py-10 lg:px-8">
          <div className="mx-auto w-full max-w-5xl">{children}</div>
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
          {section.children?.map((child, idx) => {
            const prev = idx > 0 ? section.children?.[idx - 1] : undefined;
            const showGroupHeader =
              !!child.group && child.group !== prev?.group;
            const active = isChildActive(
              pathname,
              section.key,
              section.children ?? [],
              child,
            );
            return (
              <Fragment key={`${section.key}-${child.label}`}>
                {showGroupHeader && idx > 0 && (
                  <DropdownMenuSeparator className="mx-1 my-1 bg-border-subtle" />
                )}
                {showGroupHeader && (
                  <DropdownMenuLabel className="px-3 pb-1 pt-2 text-[10px] uppercase tracking-[0.14em] text-foreground-tertiary">
                    {child.group}
                  </DropdownMenuLabel>
                )}
                <DropdownMenuItem
                  asChild
                  className={
                    active
                      ? "bg-primary text-primary-foreground focus:bg-primary focus:text-primary-foreground"
                      : "text-foreground focus:bg-paper-100 focus:text-foreground"
                  }
                >
                  <Link href={child.href}>{child.label}</Link>
                </DropdownMenuItem>
              </Fragment>
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
            {section.children?.map((child, idx) => {
              const prev = idx > 0 ? section.children?.[idx - 1] : undefined;
              const showGroupHeader =
                !!child.group && child.group !== prev?.group;
              return (
                <Fragment key={`${section.key}-${child.label}`}>
                  {showGroupHeader && (
                    <li
                      role="presentation"
                      aria-hidden="true"
                      className={`px-2 pb-0.5 text-[10px] font-semibold uppercase tracking-[0.14em] text-foreground-tertiary ${
                        idx === 0 ? "pt-1" : "pt-3"
                      }`}
                    >
                      {child.group}
                    </li>
                  )}
                  <SidebarMenuSubItem>
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
                </Fragment>
              );
            })}
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
  if (!isActiveLink(pathname, child.href)) return false;

  // Prefer the most specific sibling: if a longer sibling href also
  // matches the current pathname (e.g. "/customers/reviews" matches
  // both "/customers" and "/customers/reviews"), only the longer
  // one should be marked active.
  const moreSpecificMatch = siblings.some(
    (sibling) =>
      sibling.href !== child.href &&
      sibling.href.length > child.href.length &&
      isActiveLink(pathname, sibling.href),
  );
  return !moreSpecificMatch;
}

const canonicalChildLabelBySection: Record<string, string> = {
  catalog: "Products",
  customers: "All Customers",
  marketing: "Campaigns",
  settings: "Stores",
  support: "Tickets",
};

function getPageTitle(pathname: string | null): {
  eyebrow: string;
  title: string;
} {
  if (!pathname || pathname === "/" || pathname === "/dashboard") {
    return { eyebrow: "Overview", title: "Dashboard" };
  }
  if (pathname.startsWith("/products")) {
    return { eyebrow: "Products", title: "Products" };
  }
  if (pathname.startsWith("/orders/returns")) {
    return { eyebrow: "Operations", title: "Returns & replacements" };
  }
  if (pathname.startsWith("/orders")) {
    return { eyebrow: "Operations", title: "Orders" };
  }
  if (pathname.startsWith("/customers/reviews")) {
    return { eyebrow: "Customers", title: "Reviews" };
  }
  if (pathname.startsWith("/customers")) {
    return { eyebrow: "Relationships", title: "Customers" };
  }
  if (pathname.startsWith("/settings/stores")) {
    return { eyebrow: "Store Setup", title: "Stores" };
  }
  if (pathname.startsWith("/settings/themes")) {
    return { eyebrow: "Store", title: "Branding" };
  }
  if (pathname.startsWith("/settings/payments")) {
    return { eyebrow: "Selling", title: "Payments & Tax" };
  }
  if (pathname.startsWith("/settings/shipping")) {
    return { eyebrow: "Selling", title: "Shipping" };
  }
  if (pathname.startsWith("/settings/team")) {
    return { eyebrow: "Team & access", title: "Team" };
  }
  if (pathname.startsWith("/settings/account")) {
    return { eyebrow: "Settings", title: "Account" };
  }
  if (pathname.startsWith("/settings/domains")) {
    return { eyebrow: "Settings", title: "Custom Domains" };
  }
  if (pathname.startsWith("/settings/subscription")) {
    return { eyebrow: "Settings", title: "Subscription" };
  }
  if (pathname.startsWith("/settings/audit-logs")) {
    return { eyebrow: "Settings", title: "Audit Logs" };
  }
  if (pathname.startsWith("/settings/notifications")) {
    return { eyebrow: "Settings", title: "Notifications" };
  }
  if (pathname.startsWith("/marketing/coupons")) {
    return { eyebrow: "Marketing", title: "Coupons" };
  }
  if (pathname.startsWith("/marketing/gift-cards")) {
    return { eyebrow: "Marketing", title: "Gift Cards" };
  }
  if (pathname.startsWith("/marketing/loyalty")) {
    return { eyebrow: "Marketing", title: "Loyalty" };
  }
  if (pathname.startsWith("/marketing/segments")) {
    return { eyebrow: "Marketing", title: "Segments" };
  }
  if (pathname.startsWith("/marketing")) {
    return { eyebrow: "Growth", title: "Marketing" };
  }
  if (pathname.startsWith("/settings")) {
    return { eyebrow: "Configuration", title: "Settings" };
  }
  if (pathname.startsWith("/support/live-chat")) {
    return { eyebrow: "Support", title: "Live chat" };
  }
  if (pathname.startsWith("/support/audit-log")) {
    return { eyebrow: "Support", title: "Audit log" };
  }
  if (pathname.startsWith("/support/tickets")) {
    return { eyebrow: "Support", title: "Tickets" };
  }
  if (pathname.startsWith("/support/help")) {
    return { eyebrow: "Support", title: "Help Center" };
  }
  return { eyebrow: "Workspace", title: "Admin" };
}
