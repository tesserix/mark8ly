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
  const { byHeight, byWidth } = fitPercents(value, font);
  return `min(${byHeight.toFixed(2)}cqh, ${byWidth.toFixed(2)}cqw)`;
}

/** The two limits, as percentages of the print area's height and width. */
function fitPercents(value: string, font: string | undefined) {
  const lines = value.split("\n");
  const longest = Math.max(1, ...lines.map((l) => l.length));
  return { lines, byHeight: 70 / lines.length, byWidth: 92 / (longest * personalisationFontAdvance(font)) };
}

// How far the ends of a line rise above its middle, as a fraction of the
// print area's width: a mug seen from a little above.
const CURVE_RISE = 0.06;

/**
 * The text on a curved surface (a mug, bottle or candle), for an SVG of
 * the print area's pixel size: the same font size as the flat preview,
 * and one quadratic curve per line for a <textPath> to follow, ends
 * raised so the line wraps round the surface. Following a path keeps the
 * font's own letter spacing.
 */
export function curvedTextLayout(value: string, font: string | undefined, w: number, h: number) {
  const { lines, byHeight, byWidth } = fitPercents(value, font);
  const fontSize = Math.min((byHeight / 100) * h, (byWidth / 100) * w);
  const rise = CURVE_RISE * w;
  const lineHeight = fontSize * 1.1;
  const top = (h - lineHeight * lines.length) / 2;
  const f = (n: number) => Number(n.toFixed(2));
  return {
    fontSize,
    lines: lines.map((_, i) => {
      // Baseline about 0.8em into the line box; the middle of the curve
      // sits there and the ends rise above it.
      const mid = top + lineHeight * i + fontSize * 0.8;
      // A quadratic's midpoint is (y0 + 2·cy + y1) / 4, so a control point
      // as far below as the ends are above puts the middle exactly on it.
      return `M0 ${f(mid - rise)} Q${f(w / 2)} ${f(mid + rise)} ${f(w)} ${f(mid - rise)}`;
    }),
  };
}

/**
 * How the buyer's text sits on the photo so it reads as printed ink, not a
 * sticker. Dark ink multiplies into the surface, picking up its grain and
 * shading; light ink would vanish that way, so it sits on top, slightly
 * sheer. No parseable hex means default (dark) ink.
 */
export function textInk(hex: string | undefined): { mixBlendMode: "multiply" | "normal"; opacity: number } {
  const h = hex?.match(/^#?([0-9a-f]{6})$/i)?.[1];
  if (!h) return { mixBlendMode: "multiply", opacity: 0.92 };
  const lin = (i: number) => {
    const c = parseInt(h.slice(i, i + 2), 16) / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  };
  const [r, g, b] = [lin(0), lin(2), lin(4)];
  return 0.2126 * r + 0.7152 * g + 0.0722 * b > 0.4
    ? { mixBlendMode: "normal", opacity: 0.85 }
    : { mixBlendMode: "multiply", opacity: 0.92 };
}
