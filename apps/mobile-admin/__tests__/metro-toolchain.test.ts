// Metro must transform this app with THIS app's Expo toolchain.
//
// This is the same monorepo-hoisting bug the moduleNameMapper block in
// jest.config.js documents seven times over, in its eighth disguise — and the
// first one to hit the release build rather than the test run.
//
// `@sentry/react-native` hoists to the monorepo root, where mobile-storefront's
// Expo SDK 52 lives. `getSentryExpoConfig(projectRoot)` does a bare
// `require('expo/metro-config')` from its own location, so it handed Metro the
// SDK 52 transformer — whose hermes-parser (0.23.1) predates Flow's `match`
// expression. The bundle then died inside React Native 0.85's own
// VirtualView.js with "';' expected", a parse error in a file nobody here
// wrote. metro.config.js now passes this app's `getDefaultConfig` explicitly.
//
// Asserting on the resolved paths rather than on a successful bundle keeps this
// cheap: a full `expo export` takes minutes, and the paths are the actual
// failure surface. Both entries matter — NativeWind wraps `transformerPath`
// with its own root-hoisted worker and stashes the real one in
// `cssInterop_transformerPath`, so checking only the outer path proves nothing.

// tsconfig scopes `types` to ["jest"], so use require() and declare the Node
// global this file needs, matching no-touchable-opacity.test.ts.
declare const __dirname: string;

const { execFileSync } = require("child_process");
const path = require("path");

const APP_ROOT = path.resolve(__dirname, "..");
const APP_MODULES = path.join(APP_ROOT, "node_modules");

/**
 * Loads metro.config.js in a clean Node process and reports the toolchain it
 * resolved. A child process keeps Jest's own resolver — which has its own
 * hoisting workarounds — out of the answer.
 */
function resolveToolchain(): {
  babelTransformerPath: string;
  innerTransformerPath: string;
  hermesParser: string;
} {
  const probe = `
    const path = require('path');
    const cfg = require(${JSON.stringify(path.join(APP_ROOT, "metro.config.js"))});
    const t = cfg.transformer || {};
    const babelTransformerPath = t.babelTransformerPath;
    process.stdout.write(JSON.stringify({
      babelTransformerPath,
      // NativeWind moves the real worker aside; fall back for a bare config.
      innerTransformerPath: t.cssInterop_transformerPath || cfg.transformerPath,
      hermesParser: require.resolve('hermes-parser/package.json', {
        paths: [path.dirname(babelTransformerPath)],
      }),
    }));
  `;
  const out = execFileSync("node", ["-e", probe], {
    cwd: APP_ROOT,
    encoding: "utf8",
  });
  return JSON.parse(out);
}

describe("metro toolchain resolution", () => {
  const toolchain = resolveToolchain();

  it.each([
    ["babel transformer", "babelTransformerPath"],
    ["metro transform worker", "innerTransformerPath"],
  ] as const)(
    "resolves the %s inside this app, not the hoisted root",
    (_label, key) => {
      const resolved = toolchain[key];
      expect(resolved).toBeTruthy();
      expect(resolved.startsWith(APP_MODULES + path.sep)).toBe(true);
    },
  );

  it("parses React Native's own sources with a hermes-parser that understands them", () => {
    expect(toolchain.hermesParser.startsWith(APP_MODULES + path.sep)).toBe(true);

    // The far-end check: the version the transformer will actually use must
    // parse the file the release build choked on. A path assertion alone would
    // still pass if this app's own parser were ever downgraded.
    const virtualView = path.join(
      APP_MODULES,
      "react-native/src/private/components/virtualview/VirtualView.js",
    );
    const probe = `
      const fs = require('fs');
      require(${JSON.stringify(path.dirname(toolchain.hermesParser))}).parse(
        fs.readFileSync(${JSON.stringify(virtualView)}, 'utf8'),
        { babel: true, flow: 'all', sourceType: 'module' },
      );
    `;
    expect(() =>
      execFileSync("node", ["-e", probe], { cwd: APP_ROOT, encoding: "utf8" }),
    ).not.toThrow();
  });
});
