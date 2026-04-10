"use client";

import * as React from "react";
import { useState } from "react";
import { MoreHorizontal } from "lucide-react";
import type { AdminMediaResponse } from "@/lib/api/marketplace-api";

export type MediaAction = "set-primary" | "crop" | "replace" | "delete" | "edit-alt";

export interface MediaCardProps {
  media: AdminMediaResponse;
  isPrimary: boolean;
  onAction: (action: MediaAction, media: AdminMediaResponse) => void;
}

const MENU: ReadonlyArray<{ action: MediaAction; label: string }> = [
  { action: "set-primary", label: "Set as primary" },
  { action: "crop", label: "Crop" },
  { action: "replace", label: "Replace" },
  { action: "edit-alt", label: "Edit alt text" },
  { action: "delete", label: "Delete" },
];

export function MediaCard({ media, isPrimary, onAction }: MediaCardProps): React.ReactElement {
  const [menuOpen, setMenuOpen] = useState<boolean>(false);

  return (
    <div
      className="group relative h-40 w-40 overflow-hidden rounded-[6px] border border-[var(--ink-100)] bg-[var(--background-elevated)] shadow-[var(--shadow-1)] transition-shadow hover:shadow-[var(--shadow-2)]"
      data-primary={isPrimary ? "true" : undefined}
    >
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img
        src={media.url}
        alt={media.alt ?? ""}
        className="h-full w-full object-cover"
        draggable={false}
      />

      {isPrimary ? (
        <span
          aria-label="Primary image"
          className="absolute left-2 top-2 flex h-7 w-7 items-center justify-center rounded-full bg-[var(--moss-700)] font-[var(--font-serif)] text-sm text-[var(--paper-200)]"
        >
          1
        </span>
      ) : null}

      <button
        type="button"
        aria-label="Image actions"
        aria-haspopup="menu"
        aria-expanded={menuOpen}
        onClick={() => setMenuOpen((s) => !s)}
        className="absolute right-2 top-2 flex h-7 w-7 items-center justify-center rounded-[6px] bg-[var(--background-elevated)]/90 text-[var(--ink-700)] opacity-0 shadow-[var(--shadow-1)] transition-opacity focus:opacity-100 focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] group-hover:opacity-100 group-focus-within:opacity-100"
      >
        <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
      </button>

      {menuOpen ? (
        <div
          role="menu"
          className="absolute right-2 top-11 z-10 flex min-w-[10rem] flex-col rounded-[6px] border border-[var(--ink-100)] bg-[var(--background-elevated)] py-1 shadow-[var(--shadow-2)]"
        >
          {MENU.map(({ action, label }) => (
            <button
              key={action}
              type="button"
              role="menuitem"
              onClick={() => {
                setMenuOpen(false);
                onAction(action, media);
              }}
              className="px-3 py-1.5 text-left text-sm text-[var(--ink-900)] hover:bg-[var(--paper-200)] focus:bg-[var(--paper-200)] focus:outline-none"
            >
              {label}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}
