// Sizing the buyer's text inside a print area. Pure, so it is testable
// without next/font (lib/personalisation-fonts.ts), which only loads in a
// Next build.

// Average glyph advance as a fraction of the font size, per catalogue id.
// Only used so a long name shrinks to fit its print area instead of
// spilling out of it; it does not need to be exact, only in the right
// direction per font (Bebas is condensed, Courier is monospace).
const ADVANCE: Record<string, number> = {
  classic: 0.56,
  modern: 0.62,
  script: 0.42,
  handwritten: 0.45,
  block: 0.4,
  typewriter: 0.6,
};

export function personalisationFontAdvance(id: string | undefined): number {
  return (id && ADVANCE[id]) || 0.56;
}

/**
 * Font size for the text, as a CSS length against the print area.
 *
 * The print area is a size container, so cqw/cqh are percentages of IT.
 * The size is the smaller of "fill the height, split between the lines"
 * and "fit the longest line across the width", which shrinks a long name
 * to fit rather than letting it spill out of where it will be printed.
 */
export function fitTextSize(value: string, font: string | undefined): string {
  const lines = value.split("\n");
  const longest = Math.max(1, ...lines.map((l) => l.length));
  const byHeight = 70 / lines.length;
  const byWidth = 92 / (longest * personalisationFontAdvance(font));
  return `min(${byHeight.toFixed(2)}cqh, ${byWidth.toFixed(2)}cqw)`;
}
