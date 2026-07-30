// tsconfig scopes `types` to ["jest"], so Node's ambient globals are not
// available — inline `require` + a `__dirname` declaration, matching
// rollout-invariants.test.ts and product-detail-sections.test.tsx.
declare const __dirname: string;

const fs = require("fs");
const path = require("path");
const APP_ROOT: string = path.join(__dirname, "..");
const APP_DIR: string = path.join(APP_ROOT, "app");

function walk(dir: string): string[] {
  const out: string[] = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true }) as Array<{
    name: string;
    isDirectory(): boolean;
  }>) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) out.push(...walk(full));
    else if (entry.name === "_layout.tsx") out.push(full);
  }
  return out;
}

/**
 * DERIVED corpus, not a hand-written list — a layout added tomorrow is covered
 * with no edit here. That is this increment's own recorded lesson: "a coverage
 * test with a hand-copied list is not a coverage test."
 *
 * The rule: a layout that renders a <Stack> AND owns a sibling index.tsx must
 * declare an anchor. Without one, entering that stack at a nested route (the
 * Dashboard's NEEDS YOU queue, notifications.tsx, a push-payload deep link, or
 * any external mark8ly-admin:// link) leaves the stack holding only the nested
 * route — so Back exits the tab and the list screen becomes unreachable.
 *
 * Layouts WITHOUT a sibling index.tsx are correctly exempt: `more/settings` owns
 * no index and its screens are leaves reached through the More menu, which
 * anchors instead. The ROOT app/_layout.tsx is also exempt — it owns no index
 * and its children are (tabs)/login/notifications.
 */
const STACK_LAYOUTS_WITH_INDEX = walk(APP_DIR).filter((file) => {
  const src = fs.readFileSync(file, "utf8") as string;
  const rendersStack = /<Stack[\s/>]/.test(src);
  const hasIndexSibling = fs.existsSync(path.join(path.dirname(file), "index.tsx"));
  return rendersStack && hasIndexSibling;
});

describe("every tab stack that owns an index route declares it as the anchor", () => {
  it("found the stack layouts to check (guards against the corpus silently emptying)", () => {
    // A derived corpus that resolves to [] would make every assertion below
    // vacuous — the exact "test that cannot fail" shape this repo has already
    // been bitten by. Pin a floor instead.
    expect(STACK_LAYOUTS_WITH_INDEX.length).toBeGreaterThanOrEqual(5);
  });

  it.each(STACK_LAYOUTS_WITH_INDEX.map((f) => [path.relative(APP_ROOT, f), f]))(
    "%s anchors to index",
    (_label: string, file: string) => {
      const src = fs.readFileSync(file, "utf8") as string;
      expect(src).toMatch(/export\s+const\s+unstable_settings\s*=/);
      expect(src).toMatch(/initialRouteName:\s*['"]index['"]/);
    },
  );
});
