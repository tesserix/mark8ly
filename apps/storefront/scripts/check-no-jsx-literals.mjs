#!/usr/bin/env node
/**
 * Guardrail: no JSX text literals in storefront layout files.
 *
 * The storefront layout refactor (2026-04-15) reduced each layout to
 * `hero + SectionsRenderer` — all content must come from the merchant
 * or a layout's `.defaults.ts` recipe, never from hard-coded JSX text.
 *
 * This script is a pragmatic stand-in for
 * `react/jsx-no-literals` until apps/storefront adopts an ESLint
 * flat config (Next 16 removed `next lint`). It matches any JSX text
 * node that isn't whitespace / punctuation / an entity. The existing
 * approved patterns (self-closing wrappers, expression children,
 * comments) all pass.
 *
 * Runs via `npm run -w apps/storefront lint:layouts`.
 */
import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";

const LAYOUTS_DIR = "components/layouts";

// Match JSX text nodes: stuff between `>` and `<` that isn't just
// whitespace, punctuation, or a JSX expression opener. We strip
// JSX expressions, template literals, and attribute strings first so
// the remaining gaps between tags represent pure JSX text children.
function stripBalancedBraces(src) {
  let out = "";
  let depth = 0;
  for (const ch of src) {
    if (ch === "{") {
      depth += 1;
      continue;
    }
    if (ch === "}") {
      if (depth > 0) depth -= 1;
      continue;
    }
    if (depth === 0) out += ch;
  }
  return out;
}

function stripExpressionsAndStrings(src) {
  let out = stripBalancedBraces(src);
  out = out.replace(/`[^`]*`/g, "``");
  out = out.replace(/"[^"]*"/g, "\"\"");
  out = out.replace(/'[^']*'/g, "''");
  return out;
}

function stripBlockComments(src) {
  return src.replace(/\/\*[\s\S]*?\*\//g, "");
}

function findJsxLiterals(src) {
  const sanitized = stripExpressionsAndStrings(stripBlockComments(src));
  const hits = [];
  // Look for `>` followed by non-tag content before the next `<`.
  const re = />([^<>]+)</g;
  let m;
  while ((m = re.exec(sanitized)) !== null) {
    const text = m[1];
    const trimmed = text.trim();
    if (!trimmed) continue;
    // Ignore pure punctuation / entity sequences.
    if (/^[\s.,;:!?·—–-]+$/.test(trimmed)) continue;
    if (/^&[a-zA-Z]+;$/.test(trimmed)) continue;
    hits.push(trimmed.slice(0, 80));
  }
  return hits;
}

// Scope: the 8 `<Name>Layout.tsx` files. `shared.tsx` hosts shared
// primitives (still used by e.g. HeroSection styling) and `index.tsx`
// is the dispatcher — both legitimately contain JSX that is not
// merchant-facing copy, so they are out of scope.
const files = readdirSync(LAYOUTS_DIR).filter(
  (f) => f.endsWith("Layout.tsx"),
);

let failed = false;
for (const file of files) {
  const full = join(LAYOUTS_DIR, file);
  const src = readFileSync(full, "utf8");
  const hits = findJsxLiterals(src);
  if (hits.length > 0) {
    failed = true;
    console.error(`\x1b[31m✖\x1b[0m ${full}: ${hits.length} JSX text literal(s):`);
    for (const h of hits) console.error(`    "${h}"`);
  }
}

if (failed) {
  console.error(
    "\nStorefront layout files must contain no JSX text literals — " +
      "all content flows through <HeroSection> and <SectionsRenderer>. " +
      "Move copy into the layout's .defaults.ts recipe.",
  );
  process.exit(1);
}

console.log("\x1b[32m✓\x1b[0m No JSX text literals in layouts.");
