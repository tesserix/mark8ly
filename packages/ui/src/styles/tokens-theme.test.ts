import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";
import { join } from "node:path";

const css = readFileSync(join(__dirname, "mark8ly-tokens.css"), "utf8");
const theme = css.slice(css.indexOf("@theme inline"));

// A colour token only becomes a Tailwind utility if it is aliased as
// --color-<name> inside @theme. Define --destructive without --color-destructive
// and `bg-destructive` generates NO rule: no build error, no console warning,
// just a transparent background.
//
// That is exactly what shipped. @tesserix/web 2.4.2 moved AlertDialog's confirm
// button from bg-warning to bg-destructive, and because only --warning was
// mapped, "Discard" rendered white-on-white — clickable and invisible.
describe("mark8ly tokens — semantic colours reachable as utilities", () => {
  const required = [
    "destructive",
    "destructive-foreground",
    "danger",
    "warning",
    "signal",
  ];

  it.each(required)("exposes --color-%s so bg-/text- utilities resolve", (name) => {
    expect(theme).toMatch(new RegExp(`--color-${name}\\s*:`));
  });

  it("aliases every semantic colour it defines in :root", () => {
    const root = css.slice(0, css.indexOf("@theme inline"));
    // Semantic (non-scale) colour tokens: no trailing -<number>.
    const defined = [...root.matchAll(/^\s*--([a-z-]+):\s*(?:var\(--|#)/gm)]
      .map((m) => m[1])
      .filter((n) => required.includes(n));
    for (const name of defined) {
      expect(theme, `--color-${name} missing from @theme`).toMatch(
        new RegExp(`--color-${name}\\s*:`),
      );
    }
  });
});
