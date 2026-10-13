// The fonts a merchant may offer on a text personalisation field.
//
// MUST match product.TextFonts in services/marketplace-api
// (internal/product/personalisation_text_style.go): the ids are stored on
// fields and snapshotted onto orders, and the server rejects any id that
// is not in its list. A Go test reads this file and fails if the two
// drift.
//
// The admin lists `label`; the storefront renders the buyer's preview in
// `family`, which apps/storefront/lib/personalisation-fonts.ts loads.

export const PERSONALISATION_FONT_IDS = [
  "classic",
  "modern",
  "script",
  "handwritten",
  "block",
  "typewriter",
] as const;

export type PersonalisationFontId = (typeof PERSONALISATION_FONT_IDS)[number];

export interface PersonalisationFont {
  id: PersonalisationFontId;
  /** Merchant- and buyer-facing name. */
  label: string;
  /** The webfont the storefront loads for this id. */
  family: string;
}

export const PERSONALISATION_FONTS: readonly PersonalisationFont[] = [
  { id: "classic", label: "Classic serif", family: "Playfair Display" },
  { id: "modern", label: "Modern sans", family: "Montserrat" },
  { id: "script", label: "Script", family: "Great Vibes" },
  { id: "handwritten", label: "Handwritten", family: "Caveat" },
  { id: "block", label: "Block capitals", family: "Bebas Neue" },
  { id: "typewriter", label: "Typewriter", family: "Courier Prime" },
];

export function personalisationFontLabel(id: string): string {
  return PERSONALISATION_FONTS.find((f) => f.id === id)?.label ?? id;
}

/** Same bound as the server's MaxTextFontsPerField / MaxTextColoursPerField. */
export const MAX_TEXT_FONTS = 6;
export const MAX_TEXT_COLOURS = 12;
