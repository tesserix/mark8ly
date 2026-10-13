"use client";

// Fonts and colours a text field offers the buyer.
//
// Fonts come from a fixed catalogue (@repo/ui/personalisation-fonts) that
// the storefront hosts and the server validates, so this is a set of
// toggles, not a free-text family name. Order matters: the first ticked
// font, and the first colour, is what a buyer gets if they never touch the
// pickers, and what their preview shows before they do.
//
// Colours are the merchant's own named swatches. The name is what goes on
// the order and the packing slip; the hex is only how the storefront's
// preview approximates it.

import { useState } from "react";
import { Trash2 } from "lucide-react";

import {
  MAX_TEXT_COLOURS,
  PERSONALISATION_FONTS,
} from "@repo/ui/personalisation-fonts";

import type { PersonalisationTextColour } from "@/lib/api/marketplace-api";

import { inputClass } from "./kinds";

export interface TextStyleConfigProps {
  fonts: readonly string[];
  colours: readonly PersonalisationTextColour[];
  busy?: boolean;
  onSave: (patch: { text_fonts: string[]; text_colours: PersonalisationTextColour[] }) => void;
}

const HEX = /^#[0-9A-Fa-f]{6}$/;

export function TextStyleConfig({
  fonts: savedFonts,
  colours: savedColours,
  busy = false,
  onSave,
}: TextStyleConfigProps): React.ReactElement {
  const [fonts, setFonts] = useState<string[]>([...savedFonts]);
  const [colours, setColours] = useState<PersonalisationTextColour[]>([...savedColours]);
  const [draft, setDraft] = useState<PersonalisationTextColour>({ label: "", hex: "#C9A227" });

  // Ticking appends, so the order the merchant ticks in is the order the
  // buyer sees — and the first one is the default.
  function toggleFont(id: string): void {
    setFonts((cur) => (cur.includes(id) ? cur.filter((f) => f !== id) : [...cur, id]));
  }

  const draftLabel = draft.label.trim();
  const draftDuplicate = colours.some((c) => c.label.toLowerCase() === draftLabel.toLowerCase());
  const canAddColour =
    draftLabel !== "" && HEX.test(draft.hex) && !draftDuplicate && colours.length < MAX_TEXT_COLOURS;

  function addColour(): void {
    if (!canAddColour) return;
    setColours((cur) => [...cur, { label: draftLabel, hex: draft.hex.toUpperCase() }]);
    setDraft({ label: "", hex: draft.hex });
  }

  const dirty =
    JSON.stringify(fonts) !== JSON.stringify(savedFonts) ||
    JSON.stringify(colours) !== JSON.stringify(savedColours);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <div className="flex flex-col gap-1">
          <span className="text-xs font-medium text-[var(--ink-700)]">Fonts the buyer can choose</span>
          <p className="text-xs text-[var(--ink-500)]">
            Only offer fonts you can produce. The first one you tick is the default. Leave all
            unticked to use your own house style.
          </p>
        </div>
        <div className="flex flex-wrap gap-2" role="group" aria-label="Fonts">
          {PERSONALISATION_FONTS.map((f) => {
            const position = fonts.indexOf(f.id);
            const on = position !== -1;
            return (
              <button
                key={f.id}
                type="button"
                aria-pressed={on}
                onClick={() => toggleFont(f.id)}
                className={`flex items-center gap-1.5 rounded border px-2.5 py-1 text-xs focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] ${
                  on
                    ? "border-[var(--moss-700)] bg-[var(--moss-700)]/10 text-[var(--ink-900)]"
                    : "border-[var(--ink-200)] text-[var(--ink-700)]"
                }`}
              >
                {on && <span className="font-mono text-[10px] text-[var(--moss-700)]">{position + 1}</span>}
                {f.label}
              </button>
            );
          })}
        </div>
      </div>

      <div className="flex flex-col gap-2">
        <div className="flex flex-col gap-1">
          <span className="text-xs font-medium text-[var(--ink-700)]">Colours the buyer can choose</span>
          <p className="text-xs text-[var(--ink-500)]">
            Name each one as you&apos;d produce it, like &ldquo;Gold foil&rdquo;. The swatch is only
            how the buyer&apos;s preview shows it. The first colour is the default.
          </p>
        </div>

        {colours.length > 0 && (
          <ul className="flex flex-col gap-1.5">
            {colours.map((c, i) => (
              <li key={c.label} className="flex items-center gap-2 text-sm">
                <span
                  aria-hidden
                  className="h-4 w-4 shrink-0 rounded-full border border-[var(--ink-200)]"
                  style={{ backgroundColor: c.hex }}
                />
                <span className="text-[var(--ink-900)]">{c.label}</span>
                <span className="font-mono text-xs text-[var(--ink-500)]">{c.hex}</span>
                {i === 0 && (
                  <span className="text-[10px] uppercase tracking-widest text-[var(--moss-700)]">Default</span>
                )}
                <button
                  type="button"
                  onClick={() => setColours((cur) => cur.filter((x) => x.label !== c.label))}
                  aria-label={`Remove ${c.label}`}
                  className="ml-auto text-[var(--ink-500)] hover:text-[color:var(--signal,#C23B22)]"
                >
                  <Trash2 aria-hidden size={14} />
                </button>
              </li>
            ))}
          </ul>
        )}

        {colours.length < MAX_TEXT_COLOURS && (
          <div className="flex flex-wrap items-center gap-2">
            <input
              type="color"
              value={HEX.test(draft.hex) ? draft.hex : "#000000"}
              onChange={(e) => setDraft((d) => ({ ...d, hex: e.target.value.toUpperCase() }))}
              aria-label="Swatch colour"
              className="h-9 w-10 cursor-pointer rounded border border-[var(--ink-200)] bg-transparent p-0.5"
            />
            <input
              type="text"
              value={draft.label}
              onChange={(e) => setDraft((d) => ({ ...d, label: e.target.value }))}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  addColour();
                }
              }}
              placeholder="Colour name, e.g. Gold foil"
              aria-label="Colour name"
              maxLength={60}
              className={`${inputClass} max-w-xs`}
            />
            <button
              type="button"
              onClick={addColour}
              disabled={!canAddColour}
              className="text-xs uppercase tracking-widest text-[var(--moss-700)] disabled:opacity-40"
            >
              + Add colour
            </button>
            {draftDuplicate && draftLabel !== "" && (
              <span className="text-xs text-[color:var(--signal,#C23B22)]">That name is already used.</span>
            )}
          </div>
        )}
      </div>

      <div>
        <button
          type="button"
          disabled={busy || !dirty}
          onClick={() => onSave({ text_fonts: fonts, text_colours: colours })}
          className="rounded-md bg-[var(--ink-900)] px-3 py-1.5 text-xs text-[var(--paper-200)] disabled:opacity-40"
        >
          Save fonts and colours
        </button>
      </div>
    </div>
  );
}
