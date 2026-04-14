"use client";

// apps/storefront/components/CustomerAccountMenu.tsx
//
// Auth-aware account menu for the nav bar. Shows "Sign in" when
// unauthenticated, or a dropdown with account links + sign out
// when authenticated. Uses Paper · Ink · Moss design tokens.

import { useState, useRef, useEffect, useCallback } from "react";
import Link from "next/link";
import { useCustomerAuth } from "./CustomerAuthProvider";

const linkClass =
  "block w-full px-4 py-2 text-left text-sm text-[color:var(--ink-900)] opacity-70 transition-opacity hover:opacity-100 hover:bg-[color:var(--paper-200)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]";

const MENU_ITEMS = [
  { href: "/account", label: "My account" },
  { href: "/account/orders", label: "Orders" },
  { href: "/account/gift-cards", label: "Gift cards" },
  { href: "/account/addresses", label: "Addresses" },
] as const;

export function CustomerAccountMenu() {
  const { isAuthenticated, displayName, loginUrl, logoutUrl } =
    useCustomerAuth();
  const [open, setOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  const menuContainerRef = useRef<HTMLDivElement>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);

  const close = useCallback(() => setOpen(false), []);

  /** Collect all focusable menuitems inside the dropdown. */
  const getMenuItems = useCallback((): HTMLElement[] => {
    if (!menuContainerRef.current) return [];
    return Array.from(
      menuContainerRef.current.querySelectorAll<HTMLElement>('[role="menuitem"]')
    );
  }, []);

  /** Focus a menuitem by index, clamped to valid range. */
  const focusItem = useCallback(
    (index: number) => {
      const items = getMenuItems();
      if (items.length === 0) return;
      const clamped = Math.max(0, Math.min(index, items.length - 1));
      items[clamped]?.focus();
    },
    [getMenuItems]
  );

  // Close dropdown on outside click
  useEffect(() => {
    if (!open) return;
    function handleClick(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        close();
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [open, close]);

  // Keyboard navigation inside the menu
  useEffect(() => {
    if (!open) return;
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        close();
        buttonRef.current?.focus();
        return;
      }

      const items = getMenuItems();
      if (items.length === 0) return;

      const active = document.activeElement as HTMLElement;
      const currentIndex = items.indexOf(active);

      switch (e.key) {
        case "ArrowDown": {
          e.preventDefault();
          const next =
            currentIndex < 0 ? 0 : (currentIndex + 1) % items.length;
          items[next]?.focus();
          break;
        }
        case "ArrowUp": {
          e.preventDefault();
          const prev =
            currentIndex <= 0 ? items.length - 1 : currentIndex - 1;
          items[prev]?.focus();
          break;
        }
        case "Home": {
          e.preventDefault();
          items[0]?.focus();
          break;
        }
        case "End": {
          e.preventDefault();
          items[items.length - 1]?.focus();
          break;
        }
      }
    }
    document.addEventListener("keydown", handleKey);
    return () => document.removeEventListener("keydown", handleKey);
  }, [open, close, getMenuItems]);

  // Focus first menuitem when menu opens
  useEffect(() => {
    if (open) {
      // Defer to allow the hidden class to be removed and elements to render
      requestAnimationFrame(() => focusItem(0));
    }
  }, [open, focusItem]);

  if (!isAuthenticated) {
    // `lib/auth.ts` returns "#" as the loginUrl when `AUTH_BFF_URL` is
    // misconfigured for production. Rendering a dead anchor would give
    // customers a Sign in link that silently does nothing — hide it
    // instead so the nav at least looks intentional.
    if (!loginUrl || loginUrl === "#") {
      return null;
    }
    return (
      <a
        href={loginUrl}
        rel="noopener"
        className="text-[color:var(--ink-900)] opacity-70 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        Sign in
      </a>
    );
  }

  const initial = displayName?.charAt(0).toUpperCase() ?? "A";

  return (
    <div ref={menuRef} className="relative">
      <button
        ref={buttonRef}
        type="button"
        onClick={() => setOpen((prev) => !prev)}
        aria-expanded={open}
        aria-haspopup="true"
        aria-label={`Account menu for ${displayName ?? "your account"}`}
        className="flex h-8 w-8 items-center justify-center rounded-full bg-[color:var(--moss-700)] text-xs font-medium text-white transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        {initial}
      </button>

      <div
        ref={menuContainerRef}
        role="menu"
        className={[
          "absolute right-0 top-full z-50 mt-2 w-48 overflow-hidden rounded-[6px] bg-white shadow-[var(--shadow-2,0_4px_12px_rgba(0,0,0,0.08))] border border-[color:var(--ink-900)]/10",
          open ? "" : "hidden",
        ].join(" ")}
      >
        {displayName && (
          <div className="border-b border-[color:var(--ink-900)]/10 px-4 py-2">
            <p className="truncate text-sm font-medium text-[color:var(--ink-900)]">
              {displayName}
            </p>
          </div>
        )}

        {MENU_ITEMS.map((item) => (
          <Link
            key={item.href}
            href={item.href}
            role="menuitem"
            tabIndex={open ? 0 : -1}
            className={linkClass}
            onClick={close}
          >
            {item.label}
          </Link>
        ))}

        <div className="border-t border-[color:var(--ink-900)]/10">
          <a
            href={logoutUrl}
            role="menuitem"
            tabIndex={open ? 0 : -1}
            className={linkClass}
          >
            Sign out
          </a>
        </div>
      </div>
    </div>
  );
}
