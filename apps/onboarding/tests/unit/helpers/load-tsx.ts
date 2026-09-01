import { readFileSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import ts from "typescript";

/**
 * Loads a named export from a real `.tsx` source file, with its JSX
 * compiled by the TypeScript compiler API rather than Playwright's own
 * transform.
 *
 * tests/unit runs under @playwright/test as plain Node (see
 * ../../playwright.unit.config.ts). That runner instruments every
 * non-node_modules `.ts`/`.tsx` file it loads with its own JSX
 * transform, whose `jsx-runtime` is Playwright's own (built for its
 * ARIA-snapshot matchers) rather than React's — importing a component
 * directly would silently swap in Playwright's `jsx()` and produce inert
 * `{__pw_type: "jsx", ...}` objects instead of real React elements,
 * which `react-dom/server`'s `renderToStaticMarkup` then rejects with
 * "Objects are not valid as a React child". Transpiling the source
 * ourselves into plain CommonJS leaves no JSX syntax for Playwright's
 * loader to intercept.
 *
 * Factored out of tests/unit/mail-link.spec.ts, which had the original,
 * single-use version of this loader — pulled into a shared helper now
 * that journal-signup-fields.spec.tsx needs the same trick.
 */
export function loadTsxExport<T>(
  sourcePath: string,
  exportName: string,
  outputDir: string,
): T {
  const source = readFileSync(sourcePath, "utf8");
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: {
      jsx: ts.JsxEmit.ReactJSX,
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2020,
      esModuleInterop: true,
    },
    fileName: path.basename(sourcePath),
  });

  // Written next to the calling spec (outputDir, its __dirname) rather
  // than os.tmpdir() so the generated file's `require("react/jsx-
  // runtime")` resolves via the normal node_modules ancestor walk.
  const base = path.basename(sourcePath, path.extname(sourcePath));
  const generatedPath = path.join(outputDir, `.${base}.generated.cjs`);
  writeFileSync(generatedPath, outputText);
  try {
    delete require.cache[require.resolve(generatedPath)];
    // eslint-disable-next-line @typescript-eslint/no-var-requires
    return require(generatedPath)[exportName] as T;
  } finally {
    rmSync(generatedPath, { force: true });
  }
}
