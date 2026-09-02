import { readFileSync, rmSync, writeFileSync } from "node:fs";
import { expect, test } from "@playwright/test";
import path from "node:path";
import ts from "typescript";
import { createElement, type ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { loadTsxExport } from "./helpers/load-tsx";

/**
 * JournalUnsubscribeConfirm is a "use client" component, but
 * renderToStaticMarkup happily renders it server-side too (no effects
 * run, hooks just resolve their initial state) — see
 * tests/unit/helpers/load-tsx.ts for why the real .tsx source is loaded
 * this way instead of a plain import.
 */
type JournalUnsubscribeConfirmProps = { token: string | null };

const componentsMarketingDir = path.join(
  __dirname,
  "../../components/marketing",
);
const fieldsSourcePath = path.join(
  componentsMarketingDir,
  "JournalUnsubscribeFields.tsx",
);
const confirmSourcePath = path.join(
  componentsMarketingDir,
  "JournalUnsubscribeConfirm.tsx",
);

/**
 * JournalUnsubscribeConfirm's compiled output does `require("./JournalUnsubscribeFields")` —
 * a *sibling* .tsx file, not a package. loadTsxExport handles loading a
 * single leaf component (see its own doc comment for why Playwright's
 * built-in JSX transform can't be trusted), but it deletes its generated
 * file immediately after requiring it, so it can't double as a
 * resolvable sibling module for another generated file's `require()`.
 *
 * Node's module resolution tries the exact ".js" extension before ever
 * consulting Playwright's ".tsx" loader hook, so writing a real,
 * non-hidden "JournalUnsubscribeFields.js" next to the source (rather
 * than loadTsxExport's hidden, load-and-delete ".*.generated.cjs")
 * makes plain `require("./JournalUnsubscribeFields")` resolve to our
 * transpiled shim instead of tripping Playwright's own JSX pragma. It's
 * removed again immediately after loadTsxExport has finished requiring
 * Confirm below — by then Node has already loaded and cached it.
 */
function writeFieldsShim(): () => void {
  const shimPath = path.join(
    componentsMarketingDir,
    "JournalUnsubscribeFields.js",
  );
  const source = readFileSync(fieldsSourcePath, "utf8");
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: {
      jsx: ts.JsxEmit.ReactJSX,
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2020,
      esModuleInterop: true,
    },
    fileName: "JournalUnsubscribeFields.tsx",
  });
  writeFileSync(shimPath, outputText);
  return () => rmSync(shimPath, { force: true });
}

const removeFieldsShim = writeFieldsShim();
let JournalUnsubscribeConfirm: (
  props: JournalUnsubscribeConfirmProps,
) => ReactNode;
try {
  JournalUnsubscribeConfirm = loadTsxExport<
    (props: JournalUnsubscribeConfirmProps) => ReactNode
  >(confirmSourcePath, "JournalUnsubscribeConfirm", componentsMarketingDir);
} finally {
  removeFieldsShim();
}

function stubFetchCounter(): { calls: number } {
  const counter = { calls: 0 };
  globalThis.fetch = (async () => {
    counter.calls += 1;
    return {
      ok: true,
      status: 200,
      json: async () => ({ ok: true }),
    };
  }) as unknown as typeof fetch;
  return counter;
}

// THE prefetch-hazard guard: mail clients and security scanners prefetch
// links, including this one. If rendering (or mounting) this component
// ever called the unsubscribe API on its own, a prefetch would silently
// unsubscribe someone who never clicked anything. This must only ever
// happen from a real click on the confirm button — never a useEffect, a
// render-time call, or any other form of "run this automatically".
test("rendering the confirm screen never calls the unsubscribe API on its own", () => {
  const counter = stubFetchCounter();

  renderToStaticMarkup(
    createElement(JournalUnsubscribeConfirm, { token: "a".repeat(64) }),
  );

  expect(counter.calls).toBe(0);
});

test("renders a confirm button rather than auto-submitting when a token is present", () => {
  const counter = stubFetchCounter();

  const html = renderToStaticMarkup(
    createElement(JournalUnsubscribeConfirm, { token: "a".repeat(64) }),
  );

  expect(html).toContain("<button");
  expect(counter.calls).toBe(0);
});

test("a missing token renders a message pointing at /contact instead of a confirm button", () => {
  const html = renderToStaticMarkup(
    createElement(JournalUnsubscribeConfirm, { token: null }),
  );

  expect(html).not.toContain("<button");
  expect(html).toContain("/contact");
});

test("a blank token is treated the same as a missing one", () => {
  const html = renderToStaticMarkup(
    createElement(JournalUnsubscribeConfirm, { token: "   " }),
  );

  expect(html).not.toContain("<button");
  expect(html).toContain("/contact");
});
