// The webfonts behind the personalisation preview.
//
// next/font downloads these at build time and serves them from our own
// origin, so they fall under the CSP's font-src 'self' with no third-party
// host. preload is off: only a product with a styled text field needs
// them, and the @font-face rules only fetch a file once something renders
// in that family.
//
// Keyed by the catalogue id in @repo/ui/personalisation-fonts, which a Go
// test pins to the server's list.

import {
  Bebas_Neue,
  Caveat,
  Courier_Prime,
  Great_Vibes,
  Montserrat,
  Playfair_Display,
} from "next/font/google";

import type { PersonalisationFontId } from "@repo/ui/personalisation-fonts";

const classic = Playfair_Display({ subsets: ["latin"], weight: ["600"], display: "swap", preload: false });
const modern = Montserrat({ subsets: ["latin"], weight: ["600"], display: "swap", preload: false });
const script = Great_Vibes({ subsets: ["latin"], weight: ["400"], display: "swap", preload: false });
const handwritten = Caveat({ subsets: ["latin"], weight: ["600"], display: "swap", preload: false });
const block = Bebas_Neue({ subsets: ["latin"], weight: ["400"], display: "swap", preload: false });
const typewriter = Courier_Prime({ subsets: ["latin"], weight: ["700"], display: "swap", preload: false });

const FAMILIES: Record<PersonalisationFontId, string> = {
  classic: classic.style.fontFamily,
  modern: modern.style.fontFamily,
  script: script.style.fontFamily,
  handwritten: handwritten.style.fontFamily,
  block: block.style.fontFamily,
  typewriter: typewriter.style.fontFamily,
};

const WEIGHTS: Record<PersonalisationFontId, number> = {
  classic: 600,
  modern: 600,
  script: 400,
  handwritten: 600,
  block: 400,
  typewriter: 700,
};

/**
 * Inline style for text in a catalogue font. An unknown id — a font the
 * server added before this build shipped — renders in the page's own font
 * rather than failing.
 */
export function personalisationFontStyle(id: string | undefined): React.CSSProperties {
  if (!id || !(id in FAMILIES)) return {};
  const key = id as PersonalisationFontId;
  return { fontFamily: FAMILIES[key], fontWeight: WEIGHTS[key] };
}
