"use client";

import { LogOut, Settings as SettingsIcon, User } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@tesserix/web";

interface UserMenuProps {
  email?: string;
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

/**
 * Top-right user menu. Shows a short display name derived from the
 * session email (from the middleware-forwarded headers) and exposes
 * the logout flow and a shortcut into settings. The full email is
 * still visible inside the dropdown label.
 */
export function UserMenu({ email }: UserMenuProps) {
  const initial = email?.trim().charAt(0).toUpperCase() ?? "M";
  const display = displayNameFromEmail(email);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className="inline-flex items-center gap-3 rounded-full border border-border/80 bg-white/82 px-2 py-1.5 text-sm text-muted-foreground shadow-[0_12px_26px_rgba(76,52,24,0.08)] transition-[background-color,color,border-color,box-shadow] hover:border-border hover:bg-white hover:text-foreground hover:shadow-[0_14px_30px_rgba(76,52,24,0.1)]"
          aria-label="Account menu"
          aria-haspopup="menu"
        >
          <span className="flex h-8 w-8 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground shadow-[0_8px_20px_rgba(34,28,23,0.16)]">
            {initial}
          </span>
          <span className="hidden max-w-[12rem] truncate pr-2 sm:inline">
            {display}
          </span>
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56 rounded-2xl border-border/80 bg-[rgba(255,252,248,0.98)] shadow-[0_24px_60px_rgba(76,52,24,0.14)]">
        <DropdownMenuLabel className="truncate">
          {email ?? "Not signed in"}
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild>
          <a href="/settings/account">
            <User className="mr-2 h-4 w-4" />
            Profile
          </a>
        </DropdownMenuItem>
        <DropdownMenuItem asChild>
          <a href="/settings">
            <SettingsIcon className="mr-2 h-4 w-4" />
            Settings
          </a>
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild>
          <a href="/logout" className="text-destructive focus:text-destructive">
            <LogOut className="mr-2 h-4 w-4" />
            Sign out
          </a>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
