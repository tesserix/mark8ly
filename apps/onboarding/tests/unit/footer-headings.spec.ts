import { expect, test } from "@playwright/test";
import { readFileSync } from "node:fs";
import path from "node:path";

/**
 * Regression guard for GitHub issue #603, finding 1.
 *
 * The footer's link columns ("Product", "Compare", "Resources", "Company",
 * "Legal") were marked up as <h2>. The footer renders on every page, so
 * those five words sat in the same document outline as the real content of
 * each page — and heading-based extractors, including the ones AI systems
 * use to segment a document, read them as content sections.
 *
 * The fix is not simply "delete the heading". The label is a real
 * accessibility affordance: it tells a screen-reader user which group of
 * links they are in. So the column is a <nav> landmark named by the same
 * visible text via aria-labelledby, which keeps the labelling and drops the
 * outline pollution.
 *
 * This guard pins both halves — no heading, and still labelled.
 */
const FOOTER = "components/marketing/Footer.tsx";

function source(): string {
  return readFileSync(path.join(__dirname, "../..", FOOTER), "utf8");
}

/**
 * Comments are stripped before scanning, for the same reason as
 * homepage-static-render.spec.ts: the comment above FooterColumn names the
 * <h2> in order to explain why it must not come back, and scanning raw
 * source would make documenting the fix trip the guard enforcing it.
 */
function codeOnly(text: string): string {
  return text.replace(/\/\*[\s\S]*?\*\//g, "").replace(/^\s*\/\/.*$/gm, "");
}

test("the footer declares no headings at all (#603)", () => {
  const offending = codeOnly(source())
    .split("\n")
    .map((line, i) => ({ line: line.trim(), number: i + 1 }))
    // JSX heading elements only: <h1>…<h6>. \b rather than [\s>] because a
    // multi-attribute JSX heading puts the tag name alone on its line, and
    // requiring a following space or bracket would miss exactly that shape.
    .filter(({ line }) => /<h[1-6]\b/.test(line));

  expect(
    offending,
    `${FOOTER} renders a heading element. The footer is on every page, so a ` +
      "heading here becomes a phantom content section in every page's " +
      "outline (#603). Use a <nav aria-labelledby> landmark instead.",
  ).toEqual([]);
});

test("each footer link column is still an accessibly labelled nav (#603)", () => {
  const src = source();

  expect(
    src,
    "the footer column must be a <nav> landmark so the grouping survives " +
      "the heading removal",
  ).toContain("<nav aria-labelledby=");

  // The label must come from the visible column title, not a hardcoded or
  // visually-hidden duplicate of text that is already on screen.
  expect(src).toMatch(/id=\{labelId\}/);
  expect(src).toMatch(/\{title\}/);
});
