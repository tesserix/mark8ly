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

/**
 * Top-right user menu. Shows the merchant's email (from the session
 * headers forwarded by middleware) and exposes the logout flow and a
 * shortcut into settings. Account / profile editing pages are stubs
 * for now — they link to the /settings placeholder.
 */
export function UserMenu({ email }: UserMenuProps) {
  const initial = email?.trim().charAt(0).toUpperCase() ?? "M";
  const display = email && email.length > 0 ? email : "Account";

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
          <a href="/settings">
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
