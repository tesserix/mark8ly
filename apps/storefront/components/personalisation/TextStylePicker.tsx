"use client";

// Font and colour pickers for a text personalisation field, plus the live
// preview of the buyer's words on the merchant's mockup.
//
// Both pickers are radio groups, not menus: there are at most six fonts
// and twelve colours, and seeing every option at once — each font name
// set in its own face, each colour as a swatch — is the whole point.
// Nothing is pre-selected visually that is not also what will be ordered:
// resolveTextStyle gives the same answer the server's checkout gives.

import { personalisationFontLabel } from "@repo/ui/personalisation-fonts";

import type { StorefrontPersonalisationField } from "@/lib/api/marketplace-api";
import { personalisationFontStyle } from "@/lib/personalisation-fonts";
import { resolveTextStyle, type PersonalisationAnswer } from "@/lib/personalisation";

import { MockupPreview } from "./MockupPreview";

interface TextStylePickerProps {
  field: StorefrontPersonalisationField;
  answer: PersonalisationAnswer;
  onChange: (patch: Pick<PersonalisationAnswer, "font" | "colour">) => void;
}

export function TextStylePicker({ field, answer, onChange }: TextStylePickerProps) {
  const fonts = field.text_fonts ?? [];
  const colours = field.text_colours ?? [];
  const style = resolveTextStyle(field, answer);
  const hasPreview = Boolean(field.mockup_url && field.print_area);

  if (fonts.length === 0 && colours.length === 0 && !hasPreview) return null;

  return (
    <div className="flex flex-col gap-3">
      {fonts.length > 1 && (
        <fieldset className="flex flex-col gap-1.5">
          <legend className="mb-1 text-xs font-medium text-[color:var(--storefront-text,var(--ink-900))]/80">
            Font
          </legend>
          <div className="flex flex-wrap gap-2">
            {fonts.map((id) => {
              const selected = style.font === id;
              return (
                <label
                  key={id}
                  className={`cursor-pointer rounded-md border px-3 py-1.5 text-base transition-colors has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-[color:var(--storefront-accent,var(--moss-700))] ${
                    selected
                      ? "border-[color:var(--storefront-accent,var(--moss-700))] bg-[color:var(--storefront-accent,var(--moss-700))]/[0.08]"
                      : "border-[color:var(--storefront-text,var(--ink-900))]/20 hover:border-[color:var(--storefront-text,var(--ink-900))]/40"
                  }`}
                >
                  <input
                    type="radio"
                    name={`${field.id}-font`}
                    value={id}
                    checked={selected}
                    onChange={() => onChange({ font: id })}
                    className="sr-only"
                  />
                  {/* The name in its own face: the sample IS the label. */}
                  <span style={personalisationFontStyle(id)}>{personalisationFontLabel(id)}</span>
                </label>
              );
            })}
          </div>
        </fieldset>
      )}

      {colours.length > 1 && (
        <fieldset className="flex flex-col gap-1.5">
          <legend className="mb-1 text-xs font-medium text-[color:var(--storefront-text,var(--ink-900))]/80">
            Colour: <span className="font-normal">{style.colour?.label}</span>
          </legend>
          <div className="flex flex-wrap gap-2">
            {colours.map((c) => {
              const selected = style.colour?.label === c.label;
              return (
                <label
                  key={c.label}
                  title={c.label}
                  className={`flex h-8 w-8 cursor-pointer items-center justify-center rounded-full border-2 transition-colors has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-[color:var(--storefront-accent,var(--moss-700))] ${
                    selected
                      ? "border-[color:var(--storefront-accent,var(--moss-700))]"
                      : "border-transparent"
                  }`}
                >
                  <input
                    type="radio"
                    name={`${field.id}-colour`}
                    value={c.label}
                    checked={selected}
                    onChange={() => onChange({ colour: c.label })}
                    aria-label={c.label}
                    className="sr-only"
                  />
                  <span
                    aria-hidden
                    className="h-6 w-6 rounded-full border border-[color:var(--storefront-text,var(--ink-900))]/20"
                    style={{ backgroundColor: c.hex }}
                  />
                </label>
              );
            })}
          </div>
        </fieldset>
      )}

      {/* One font or one colour is not a choice, but it is still how the
          text will be made, so say so rather than hide it. */}
      {(fonts.length === 1 || colours.length === 1) && (
        <p className="text-xs text-[color:var(--storefront-text,var(--ink-900))]/60">
          {[
            fonts.length === 1 && style.font ? personalisationFontLabel(style.font) : null,
            colours.length === 1 ? style.colour?.label : null,
          ]
            .filter(Boolean)
            .join(" · ")}
        </p>
      )}

      {hasPreview && (
        <MockupPreview
          mockupUrl={field.mockup_url!}
          printArea={field.print_area!}
          fieldLabel={field.label}
          text={{
            value: answer.text ?? "",
            font: style.font,
            colourHex: style.colour?.hex,
          }}
        />
      )}
    </div>
  );
}
