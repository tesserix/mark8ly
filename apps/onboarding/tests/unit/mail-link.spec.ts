import { expect, test } from "@playwright/test";
import { readdirSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
import path from "node:path";
import { createElement, type ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import ts from "typescript";

/**
 * MailLink is the fix for GitHub issue #147: Cloudflare's email
 * obfuscation rewrites `mailto:` anchors into a `/cdn-cgi/l/email-
 * protection` path that 404s on mark8ly.com. Wrapping the anchor in
 * `<!--email_off--> ... <!--email_on-->` opts it out of that rewrite.
 *
 * WHY THIS FILE COMPILES MailLink.tsx BY HAND
 * --------------------------------------------
 * These tests run under `tests/unit`, which is plain Node via
 * @playwright/test rather than a browser — see playwright.unit.config.ts.
 * That runner instruments every non-node_modules .ts/.tsx file it loads
 * with its own JSX transform, and that transform's `jsx-runtime` is
 * Playwright's own (built for its ARIA-snapshot matchers), not React's —
 * requiring MailLink.tsx directly would silently swap in Playwright's
 * `jsx()` and produce inert `{__pw_type: "jsx", ...}` objects instead of
 * real React elements. To exercise the actual component with real
 * react-dom, we transpile its source ourselves (via the TypeScript
 * compiler API, already a dependency) into plain CommonJS with no JSX
 * syntax left for Playwright's loader to intercept, write it to a
 * throwaway file, `require` it, then delete it.
 */
const MAIL_LINK_SOURCE_PATH = path.join(
  __dirname,
  "../../../../packages/ui/src/mail-link.tsx",
);

function loadMailLink(): (props: {
  email: string;
  className?: string;
  children?: ReactNode;
}) => ReactNode {
  const source = readFileSync(MAIL_LINK_SOURCE_PATH, "utf8");
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: {
      jsx: ts.JsxEmit.ReactJSX,
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2020,
      esModuleInterop: true,
    },
    fileName: "MailLink.tsx",
  });

  // Written next to this spec (not os.tmpdir()) so the generated file's
  // `require("react/jsx-runtime")` resolves via the normal node_modules
  // ancestor walk.
  const generatedPath = path.join(__dirname, ".mail-link.generated.cjs");
  writeFileSync(generatedPath, outputText);
  try {
    delete require.cache[require.resolve(generatedPath)];
    // eslint-disable-next-line @typescript-eslint/no-var-requires
    return require(generatedPath).MailLink;
  } finally {
    rmSync(generatedPath, { force: true });
  }
}

const MailLink = loadMailLink();

test("renders an anchor whose href is mailto:<email>", () => {
  const html = renderToStaticMarkup(
    createElement(MailLink, { email: "hello@mark8ly.com" }),
  );
  expect(html).toContain('href="mailto:hello@mark8ly.com"');
});

test("wraps the anchor in the Cloudflare email_off/email_on opt-out markers", () => {
  const html = renderToStaticMarkup(
    createElement(MailLink, { email: "hello@mark8ly.com" }),
  );

  const offIndex = html.indexOf("<!--email_off-->");
  const anchorIndex = html.indexOf("<a href=");
  const onIndex = html.indexOf("<!--email_on-->");

  expect(offIndex).toBeGreaterThan(-1);
  expect(onIndex).toBeGreaterThan(-1);
  // The markers must actually surround the anchor, not just appear
  // somewhere in the output.
  expect(offIndex).toBeLessThan(anchorIndex);
  expect(anchorIndex).toBeLessThan(onIndex);
});

test("falls back to the email address when no children are given", () => {
  const html = renderToStaticMarkup(
    createElement(MailLink, { email: "hello@mark8ly.com" }),
  );
  expect(html).toContain(">hello@mark8ly.com</a>");
});

test("renders explicit children instead of the email address", () => {
  const html = renderToStaticMarkup(
    createElement(MailLink, { email: "hello@mark8ly.com" }, "Say hello"),
  );
  expect(html).toContain(">Say hello</a>");
  expect(html).not.toContain(">hello@mark8ly.com</a>");
});

/**
 * Regression guard: nothing in apps/onboarding should ever ship a raw
 * `<a href="mailto:` anchor again — every one of them must be rendered
 * through MailLink, or Cloudflare's obfuscation 404s it. Scans .tsx source
 * under app/ and components/, skipping MailLink.tsx itself (the string
 * lives inside the component's own implementation, which is expected).
 */
test("no page ships a bare mailto anchor outside MailLink", () => {
  const appRoot = path.join(__dirname, "..", "..");
  const scanDirs = ["app", "components"];
  const offenders: string[] = [];

  const bareMailtoAnchor = /<a\s[^>]*href\s*=\s*(\{`?mailto:|"mailto:)/;

  function walk(dir: string): void {
    for (const entry of readdirSync(dir)) {
      const full = path.join(dir, entry);
      const stats = statSync(full);
      if (stats.isDirectory()) {
        if (entry === "node_modules" || entry.startsWith(".")) continue;
        walk(full);
        continue;
      }
      if (!entry.endsWith(".tsx")) continue;
      if (path.basename(full) === "MailLink.tsx") continue;

      const contents = readFileSync(full, "utf8");
      if (bareMailtoAnchor.test(contents)) {
        offenders.push(path.relative(appRoot, full));
      }
    }
  }

  for (const dir of scanDirs) {
    walk(path.join(appRoot, dir));
  }

  expect(offenders).toEqual([]);
});
