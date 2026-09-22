import { describe, expect, it } from "vitest";
import { readFileSync, readdirSync, statSync } from "node:fs";
import path from "node:path";

// #881: CSV import had never worked. Four independent defects, each
// invisible to a test that only asserted the code compiled:
//   1. client called /products/csv-imports; the route is /csv-imports
//   2. errors download asked for /errors.csv; the route is /errors
//   3. the upload passed the literal string "__STORE_ID__" as the store id
//   4. the page had no inbound link, and export had no caller at all
// 1 and 2 are pinned in csvImports.test.ts. These guard 3 and 4.

const APP = path.join(__dirname, "..", "..", "app");

function walk(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir)) {
    const full = path.join(dir, entry);
    if (statSync(full).isDirectory()) out.push(...walk(full));
    else if (/\.tsx?$/.test(entry) && !/\.test\.tsx?$/.test(entry)) out.push(full);
  }
  return out;
}

function code(src: string): string {
  return src
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .split("\n")
    .filter((l) => !l.trim().startsWith("//") && !l.trim().startsWith("*"))
    .join("\n");
}

describe("csv import wiring", () => {
  const files = walk(APP);

  it("finds app files to scan", () => {
    expect(files.length).toBeGreaterThan(20);
  });

  it("never passes a placeholder where a real store id belongs", () => {
    const offenders = files.filter((f) => code(readFileSync(f, "utf8")).includes("__STORE_ID__"));
    expect(
      offenders.map((f) => path.relative(APP, f)),
      "a literal placeholder reached an API call — resolve the store from the session instead (#881)",
    ).toEqual([]);
  });

  it("keeps the import page reachable from the products page", () => {
    const products = readFileSync(path.join(APP, "(admin)", "products", "page.tsx"), "utf8");
    expect(
      products.includes('href="/products/import"'),
      "the CSV import page must be linked from Products — it was reachable only by typing the URL (#881)",
    ).toBe(true);
  });

  it("does not let the upload action take a caller-supplied store id", () => {
    const actions = code(
      readFileSync(path.join(APP, "(admin)", "products", "import", "actions.ts"), "utf8"),
    );
    // The session is the authority on which store is being acted in.
    expect(actions).toContain("resolveStoreId");
    expect(
      /submitCsvImportAction\(\s*storeId/.test(actions),
      "submitCsvImportAction must not accept a storeId argument",
    ).toBe(false);
  });
});

// #897 follow-up: the column mapper was decorative. The page collected the
// merchant's choices into state and the submit handler built a FormData
// with only the file, so any CSV whose headers were not already the
// parser's canonical names imported nothing — and nothing failed, because
// the mapping was simply never sent.
describe("column mapping reaches the server", () => {
  const strip = (src: string) =>
    src.replace(/\/\*[\s\S]*?\*\//g, "").replace(/^\s*\/\/.*$/gm, "");

  it("the import page puts the mapping in the submitted form data", () => {
    const src = strip(
      readFileSync(
        path.join(APP, "(admin)", "products", "import", "page.tsx"),
        "utf8",
      ),
    );
    expect(src).toMatch(/formData\.append\(\s*["']column_mapping["']/);
  });

  it("the server action forwards it to the API client", () => {
    const src = strip(
      readFileSync(
        path.join(APP, "(admin)", "products", "import", "actions.ts"),
        "utf8",
      ),
    );
    expect(src).toContain("column_mapping");
    expect(src).toMatch(/submitCsvImport\([^)]*mapping/s);
  });

  it("the API client sends it as a form field", () => {
    const src = strip(readFileSync(path.join(__dirname, "csvImports.ts"), "utf8"));
    expect(src).toMatch(/formData\.append\(\s*["']column_mapping["']/);
  });
});