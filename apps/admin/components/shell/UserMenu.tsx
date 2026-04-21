"use client";

// User menu — matches the NotificationBell dropdown pattern so the
// top-right cluster reads as one visual family: icon-sized trigger
// that opens a hairline-divided panel with a header, a divided list
// of actions, and a footer link. Click-outside + Escape dismiss.

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { LogOut, Settings as SettingsIcon, User } from "lucide-react";

interface UserMenuProps {
  email?: string;
  name?: string;
  avatarUrl?: string;
}

// Pull a short display name out of the email's local part so the
// header pill shows "Mahesh" instead of a truncated "mahesh.sang...".
// Falls back to the raw local part when the local doesn't look like a
// plain name.
function displayNameFromEmail(email: string | undefined): string {
  if (!email) return "Account";
  const local = email.split("@")[0] ?? "";
  if (!local) return email;
  const first = local.split(/[+.\-_]/)[0] ?? "";
  if (!first || !/^[a-z]+$/i.test(first)) return local;
  return first.charAt(0).toUpperCase() + first.slice(1).toLowerCase();
}

export function UserMenu({ email, name, avatarUrl }: UserMenuProps) {
  const [open, setOpen] = useState(false);
  const panelRef = useRef<HTMLDivElement>(null);

  const display = (name && name.trim()) || displayNameFromEmail(email);
  const initial = display.trim().charAt(0).toUpperCase() || "M";

  // Close on outside click
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    }
    if (open) {
      document.addEventListener("mousedown", handleClickOutside);
    }
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, [open]);

  // Close on Escape
  useEffect(() => {
    function handleEscape(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }
    if (open) {
      document.addEventListener("keydown", handleEscape);
    }
    return () => document.removeEventListener("keydown", handleEscape);
  }, [open]);

  return (
    <div className="relative" ref={panelRef}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-label="Account menu"
        aria-haspopup="menu"
        aria-expanded={open}
        className="inline-flex h-11 items-center gap-2.5 rounded-full pl-1 pr-3 text-sm text-foreground-secondary transition-colors hover:bg-paper-100 hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        {avatarUrl ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={avatarUrl}
            alt=""
            aria-hidden="true"
            className="h-9 w-9 rounded-full object-cover"
          />
        ) : (
          <span
            aria-hidden="true"
            className="flex h-9 w-9 items-center justify-center rounded-full bg-[color:var(--ink-900)] text-xs font-semibold text-[color:var(--primary-foreground)]"
          >
            {initial}
          </span>
        )}
        <span className="hidden max-w-[10rem] truncate sm:inline">{display}</span>
      </button>

      {open && (
        <div
          role="menu"
          className="absolute right-0 top-full z-50 mt-2 w-72 overflow-hidden rounded-md border border-border bg-[color:var(--background-elevated)] shadow-lg"
        >
          <div className="flex items-center gap-3 border-b border-border px-4 py-3">
            {avatarUrl ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={avatarUrl}
                alt=""
                aria-hidden="true"
                className="h-10 w-10 shrink-0 rounded-full object-cover"
              />
            ) : (
              <span
                aria-hidden="true"
                className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-[color:var(--ink-900)] text-sm font-semibold text-[color:var(--primary-foreground)]"
              >
                {initial}
              </span>
            )}
            <div className="min-w-0 flex-1">
              <p className="truncate font-serif text-sm text-foreground">{display}</p>
              {email && (
                <p className="truncate text-xs text-foreground-tertiary">{email}</p>
              )}
            </div>
          </div>

          <ul role="list" className="divide-y divide-border">
            <li>
              <Link
                href="/settings/account"
                onClick={() => setOpen(false)}
                className="flex items-center gap-3 px-4 py-3 text-sm text-foreground transition-colors hover:bg-[color:var(--paper-100)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[color:var(--moss-700)]"
              >
                <User className="h-4 w-4 shrink-0 text-foreground-secondary" aria-hidden="true" />
                Profile
              </Link>
            </li>
            <li>
              <Link
                href="/settings"
                onClick={() => setOpen(false)}
                className="flex items-center gap-3 px-4 py-3 text-sm text-foreground transition-colors hover:bg-[color:var(--paper-100)] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[color:var(--moss-700)]"
              >
                <SettingsIcon className="h-4 w-4 shrink-0 text-foreground-secondary" aria-hidden="true" />
                Settings
              </Link>
            </li>
          </ul>

          <div className="border-t border-border">
            <a
              href="/logout"
              className="flex items-center gap-3 px-4 py-3 text-sm text-[color:var(--danger)] transition-colors hover:bg-[color:var(--danger)]/[0.06] focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-[color:var(--moss-700)]"
            >
              <LogOut className="h-4 w-4 shrink-0" aria-hidden="true" />
              Sign out
            </a>
          </div>
        </div>
      )}
    </div>
  );
}
