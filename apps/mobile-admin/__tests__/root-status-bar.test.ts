// tsconfig scopes `types` to ["jest"], so Node's ambient globals are NOT
// available — `import * as fs from "fs"` fails tsc with TS2591 even though jest
// runs it fine. Use inline `require` and declare `__dirname`, matching
// product-detail-sections.test.tsx and rollout-invariants.test.ts.
declare const __dirname: string;

const ROOT_LAYOUT = require("path").join(__dirname, "..", "app", "_layout.tsx");

/**
 * Guards the status-bar contrast fix.
 *
 * Nothing in this app set a status-bar style, so Android defaulted to LIGHT
 * content and drew a white clock/battery/wifi onto the Paper background
 * (#F7F6F2). Measured off two independent Pixel 8 Pro screenshots taken WITHOUT
 * SysUI demo mode: clock-region luminance 255 against a 245 background — about
 * 1.04:1, invisible on every screen in the app. iOS measured 0 (dark) on the
 * same check, so iOS was already correct and this only brings Android into line.
 *
 * This is a SOURCE assertion, and that is a deliberate, acknowledged compromise:
 * rendering `app/_layout.tsx` means standing up expo-font, expo-splash-screen,
 * the auth provider, react-query and gesture-handler, and a render test built on
 * that much mocking would assert more about the mocks than the app. The real
 * proof for this fix is on-device pixel measurement (recorded in the ledger);
 * this test exists to stop the line being deleted or flipped to "light", which
 * is the actual regression risk.
 */
describe("root layout pins dark status-bar content", () => {
  const source = require("fs").readFileSync(ROOT_LAYOUT, "utf8") as string;

  it("imports StatusBar from expo-status-bar", () => {
    expect(source).toMatch(/import\s*\{\s*StatusBar\s*\}\s*from\s*['"]expo-status-bar['"]/);
  });

  it("renders <StatusBar style=\"dark\" /> so Android stops drawing white on Paper", () => {
    expect(source).toMatch(/<StatusBar\s+style=(["']|\{['"])dark/);
  });

  it("never sets light status-bar content, which is what made it invisible", () => {
    expect(source).not.toMatch(/<StatusBar[^>]*style=(["']|\{['"])light/);
  });
});
