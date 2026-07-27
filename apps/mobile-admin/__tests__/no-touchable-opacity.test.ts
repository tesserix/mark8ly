// tsconfig scopes `types` to ["jest"], so avoid importing Node's `child_process`
// and `path` modules (no ambient types without widening the project tsconfig)
// and declare the Node global this file's __dirname resolution needs, matching
// the existing pattern in customer-detail-sections.test.tsx.
declare const __dirname: string;

const { execSync } = require("child_process");
const path = require("path");

const APP_ROOT = path.resolve(__dirname, "..");
const CORPUS = ["app", "components", "lib"];

/** Files under CORPUS whose content matches `pattern` (grep -l), name-sorted. */
function filesContaining(pattern: string): string[] {
  try {
    const out = execSync(`grep -rl "${pattern}" ${CORPUS.join(" ")}`, {
      cwd: APP_ROOT,
      encoding: "utf8",
    });
    return out.split("\n").filter(Boolean).sort();
  } catch (err: unknown) {
    // grep exits 1 (no matches, not an error) or 2 (a real problem: bad
    // pattern, unreadable path, or — the failure mode this guard exists to
    // catch — one of CORPUS having been renamed/deleted out from under it).
    // Only exit 1 means "legitimately found nothing"; anything else must
    // surface, or a renamed `app/` would silently make every assertion
    // below vacuously true.
    const status = (err as { status?: number }).status;
    if (status === 1) return [];
    throw err;
  }
}

describe("press feedback migration", () => {
  // Positive control: proves the grep-based assertions below are actually
  // searching a real, non-empty corpus rather than passing vacuously
  // because app/components/lib don't exist (e.g. after a rename). Counted
  // independently of filesContaining() so a bug in that helper can't also
  // hide a bug here.
  it("searches a non-empty corpus", () => {
    const fileCount = Number(
      execSync(
        `find ${CORPUS.join(" ")} -type f \\( -name "*.tsx" -o -name "*.ts" \\) | wc -l`,
        { cwd: APP_ROOT, encoding: "utf8" },
      ).trim(),
    );
    expect(fileCount).toBeGreaterThan(100);
  });

  it("has no remaining TouchableOpacity imports", () => {
    // Jest's own array diff on a failed toEqual([]) already lists every
    // offending file — no need for a hand-rolled message.
    expect(filesContaining("TouchableOpacity")).toEqual([]);
  });

  it("has no remaining activeOpacity props", () => {
    expect(filesContaining("activeOpacity")).toEqual([]);
  });
});
