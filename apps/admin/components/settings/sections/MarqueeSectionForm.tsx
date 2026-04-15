"use client";

import { useState, type KeyboardEvent } from "react";
import { X } from "lucide-react";

import type { HomepageSection } from "@/lib/api/marketplace-api";
import { FieldLabel } from "../BrandingSettingsClient";

type MarqueeSection = Extract<HomepageSection, { type: "marquee" }>;

interface Props {
  section: MarqueeSection;
  onChange: (next: MarqueeSection) => void;
  editable: boolean;
}

const MAX_ITEMS = 8;
const MAX_ITEM_LEN = 80;

export function MarqueeSectionForm({ section, onChange, editable }: Props) {
  const [draft, setDraft] = useState("");

  const patch = (p: Partial<MarqueeSection>) => onChange({ ...section, ...p });

  const commitDraft = () => {
    const trimmed = draft.trim();
    if (!trimmed) return;
    if (section.items.length >= MAX_ITEMS) return;
    patch({ items: [...section.items, trimmed.slice(0, MAX_ITEM_LEN)] });
    setDraft("");
  };

  const removeAt = (idx: number) =>
    patch({ items: section.items.filter((_, i) => i !== idx) });

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter" || e.key === ",") {
      e.preventDefault();
      commitDraft();
      return;
    }
    if (e.key === "Backspace" && draft === "" && section.items.length > 0) {
      removeAt(section.items.length - 1);
    }
  };

  return (
    <div className="space-y-4">
      <div className="space-y-1.5">
        <FieldLabel htmlFor="section_marquee_items">
          Items ({section.items.length}/{MAX_ITEMS})
        </FieldLabel>
        <div className="flex flex-wrap items-center gap-2 rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] p-2 focus-within:border-[color:var(--moss-700)]">
          {section.items.map((item, i) => (
            <span
              key={`${i}-${item}`}
              className="inline-flex items-center gap-1.5 rounded-md bg-[color:var(--background)] px-2 py-1 text-xs font-medium text-foreground"
            >
              {item}
              {editable ? (
                <button
                  type="button"
                  onClick={() => removeAt(i)}
                  className="text-foreground-secondary hover:text-danger"
                  aria-label={`Remove ${item}`}
                >
                  <X className="h-3 w-3" />
                </button>
              ) : null}
            </span>
          ))}
          <input
            id="section_marquee_items"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={onKeyDown}
            onBlur={commitDraft}
            disabled={!editable || section.items.length >= MAX_ITEMS}
            placeholder={
              section.items.length >= MAX_ITEMS
                ? "Maximum reached"
                : "Type and press Enter"
            }
            maxLength={MAX_ITEM_LEN}
            className="flex-1 min-w-[10rem] bg-transparent px-1 text-sm text-foreground placeholder:text-foreground-tertiary focus:outline-none disabled:opacity-50"
          />
        </div>
        <p className="text-xs text-foreground-secondary">
          Up to {MAX_ITEMS} short phrases. Press Enter or comma to add.
        </p>
      </div>

      <div className="space-y-1.5">
        <FieldLabel htmlFor="section_marquee_speed">Speed</FieldLabel>
        <select
          id="section_marquee_speed"
          value={section.speed ?? "normal"}
          onChange={(e) =>
            patch({ speed: e.target.value as MarqueeSection["speed"] })
          }
          disabled={!editable}
          className="h-10 w-40 rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] px-3 text-sm text-foreground disabled:opacity-50"
        >
          <option value="slow">Slow</option>
          <option value="normal">Normal</option>
          <option value="fast">Fast</option>
        </select>
      </div>
    </div>
  );
}
