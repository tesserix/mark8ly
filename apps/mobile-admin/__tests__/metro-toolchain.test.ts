// The app must use its Expo SDK toolchain regardless of npm's hoist layout.
declare const __dirname: string;

const { execFileSync } = require("child_process");
const path = require("path");
const APP_ROOT = path.resolve(__dirname, "..");

function resolveToolchain(): {
  babelTransformerPath: string;
  innerTransformerPath: string;
  hermesParser: string;
  expectedBabelTransformerPath: string;
  expectedInnerTransformerPath: string;
  expectedHermesParser: string;
  reactNativeRoot: string;
} {
  const probe = `
    const path = require('path');
    const { createRequire } = require('node:module');
    const expoRequire = createRequire(require.resolve('expo/package.json'));
    const sdkMetro = expoRequire('@expo/metro-config');
    const cfg = require(${JSON.stringify(path.join(APP_ROOT, "metro.config.js"))});
    const t = cfg.transformer || {};
    const babelTransformerPath = t.babelTransformerPath;
    process.stdout.write(JSON.stringify({
      babelTransformerPath,
      innerTransformerPath: t.cssInterop_transformerPath || cfg.transformerPath,
      hermesParser: require.resolve('hermes-parser/package.json', {
        paths: [path.dirname(babelTransformerPath)],
      }),
      expectedBabelTransformerPath: expoRequire.resolve('@expo/metro-config/babel-transformer'),
      expectedInnerTransformerPath: sdkMetro.unstable_transformerPath,
      expectedHermesParser: expoRequire.resolve('hermes-parser/package.json'),
      reactNativeRoot: path.dirname(require.resolve('react-native/package.json')),
    }));
  `;
  return JSON.parse(
    execFileSync("node", ["-e", probe], {
      cwd: APP_ROOT,
      encoding: "utf8",
    }),
  );
}

describe("metro toolchain resolution", () => {
  const toolchain = resolveToolchain();

  it.each([
    [
      "babel transformer",
      "babelTransformerPath",
      "expectedBabelTransformerPath",
    ],
    [
      "metro transform worker",
      "innerTransformerPath",
      "expectedInnerTransformerPath",
    ],
  ] as const)(
    "resolves the %s from this app's Expo SDK",
    (_label, actual, expected) => {
      expect(toolchain[actual]).toBe(toolchain[expected]);
    },
  );

  it.each([
    "src/private/components/virtualview/VirtualView.js",
    "Libraries/NativeComponent/ViewConfigIgnore.js",
  ])(
    "parses React Native source %s with this SDK's Hermes parser",
    (source) => {
      expect(toolchain.hermesParser).toBe(toolchain.expectedHermesParser);
      const file = path.join(toolchain.reactNativeRoot, source);
      const probe = `
      const fs = require('fs');
      require(${JSON.stringify(path.dirname(toolchain.hermesParser))}).parse(
        fs.readFileSync(${JSON.stringify(file)}, 'utf8'),
        { babel: true, flow: 'all', sourceType: 'module' },
      );
    `;
      expect(() =>
        execFileSync("node", ["-e", probe], {
          cwd: APP_ROOT,
          encoding: "utf8",
        }),
      ).not.toThrow();
    },
  );
});
