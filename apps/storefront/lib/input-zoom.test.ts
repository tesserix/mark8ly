import { readFileSync } from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

// iPhone Safari zooms into a focused field whose text is under 16px and
// stays zoomed after the keyboard closes. Many forms use text-sm (14px),
// so one rule in globals.css lifts every field to 16px on phones.
describe("phone input zoom guard", () => {
  const css = readFileSync(path.join(__dirname, "..", "app", "globals.css"), "utf8");

  it("sets every text field to 16px below the sm breakpoint", () => {
    const rule = css.match(/@media \(max-width: 639px\) \{\s*input,\s*textarea,\s*select\s*\{\s*font-size: 16px !important;/);
    expect(rule).not.toBeNull();
  });

  // Pinch zoom stays: people with low vision need it.
  it("does not disable zoom in the viewport", () => {
    const layout = readFileSync(path.join(__dirname, "..", "app", "layout.tsx"), "utf8");
    expect(layout).not.toMatch(/maximumScale|maximum-scale|userScalable|user-scalable/);
  });
});
