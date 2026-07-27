// tsconfig scopes `types` to ["jest"], so avoid importing Node's `child_process`
// and `path` modules (no ambient types without widening the project tsconfig)
// and declare the Node global this file's __dirname resolution needs, matching
// the existing pattern in customer-detail-sections.test.tsx.
declare const __dirname: string;

const { execSync } = require("child_process");
const path = require("path");

const APP_ROOT = path.resolve(__dirname, "..");

function grepCount(pattern: string): number {
  try {
    const out = execSync(
      `grep -rl "${pattern}" app components lib 2>/dev/null || true`,
      { cwd: APP_ROOT, encoding: "utf8" },
    );
    return out.split("\n").filter(Boolean).length;
  } catch {
    return 0;
  }
}

describe("press feedback migration", () => {
  it("has no remaining TouchableOpacity imports", () => {
    expect(grepCount("TouchableOpacity")).toBe(0);
  });

  it("has no remaining activeOpacity props", () => {
    expect(grepCount("activeOpacity")).toBe(0);
  });
});
